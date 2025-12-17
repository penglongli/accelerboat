// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"crypto/tls"
	syserrors "errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/apiclient"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/middleware"
)

type AccelerboatServer struct {
	op *options.AccelerBoatOption

	ginSvr      *gin.Engine
	httpServer  *http.Server
	httpSServer *http.Server
}

func NewAccelerboatServer(op *options.AccelerBoatOption) *AccelerboatServer {
	return &AccelerboatServer{
		op: op,
	}
}

func (s *AccelerboatServer) Init() error {
	return nil
}

func (s *AccelerboatServer) initHTTPRouter() {
	ginSvr := gin.New()
	ginSvr.Use(gin.Recovery())
	ginSvr.UseRawPath = true
	gin.SetMode(gin.ReleaseMode)
	ginSvr.Use(middleware.CommonMiddleware())
	s.ginSvr = ginSvr
	pprof.Register(ginSvr)
}

func (s *AccelerboatServer) Run() error {

}

func (s *AccelerboatServer) runHTTPServer(errCh chan error) {
	defer logger.Warnf("http server exit")
	serverAddr := fmt.Sprintf("0.0.0.0:%d", s.op.HTTPPort)
	s.httpServer = &http.Server{
		Addr:    serverAddr,
		Handler: s.ginSvr,
	}
	if err := s.httpServer.ListenAndServe(); err != nil && !syserrors.Is(err, http.ErrServerClosed) {
		logger.Errorf("failed to start http server: %s", err.Error())
		return
	}
	errCh <- nil
}

func (s *AccelerboatServer) runHTTPSServer(errCh chan error) {
	defer logger.Warnf("http(s) server exit")
	serverAddr := fmt.Sprintf("0.0.0.0:%d", s.op.HTTPSPort)
	tlsCerts := make([]tls.Certificate, 0)
	defaultCert := s.op.ExternalConfig.BuiltInCerts[options.LocalhostCert]
	if defaultCert == nil {
		errCh <- fmt.Errorf("not have default 'localhost' tls cert")
		return
	}
	defaultKeyPair, err := tls.X509KeyPair([]byte(defaultCert.Cert), []byte(defaultCert.Key))
	if err != nil {
		errCh <- fmt.Errorf("generate tls cert for default failed: %s", err.Error())
		return
	}
	tlsCerts = append(tlsCerts, defaultKeyPair)
	for _, mp := range s.op.ExternalConfig.RegistryMappings {
		if mp.ProxyCert == "" || mp.ProxyKey == "" {
			continue
		}
		var kp tls.Certificate
		kp, err = tls.X509KeyPair([]byte(mp.ProxyCert), []byte(mp.ProxyKey))
		if err != nil {
			errCh <- fmt.Errorf("generate tls cert for '%s' failed: %s", mp.ProxyHost, err.Error())
			return
		}
		tlsCerts = append(tlsCerts, kp)
	}
	s.httpSServer = &http.Server{
		Addr:    serverAddr,
		Handler: s.ginSvr,
		TLSConfig: &tls.Config{
			Certificates: tlsCerts,
		},
	}
	if err = s.httpSServer.ListenAndServeTLS("", ""); err != nil && !syserrors.Is(err,
		http.ErrServerClosed) {
		errCh <- err
		logger.Errorf("failed to start http(s) server: %s", err.Error())
		return
	}
	errCh <- nil
}

// Shutdown the image proxy server
func (s *AccelerboatServer) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.httpServer.Shutdown(ctx)
	s.httpSServer.Shutdown(ctx)
}

var (
	proxyHostRegex = regexp.MustCompile(`^/v[1-2]/([^/]+)/`)
)

func (s *AccelerboatServer) httpError(ctx context.Context, rw http.ResponseWriter, errMsg string, code int) {
	logger.ErrorContextf(ctx, "accelerboat server response error: %s", errMsg)
	http.Error(rw, errMsg, http.StatusBadRequest)
}

func (s *AccelerboatServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	req = apiclient.SetContext(req)
	ctx := req.Context()
	if !strings.Contains(req.RequestURI, "/custom_api/recorder") {
		logger.InfoContextf(ctx, "received request: %s, %s%s", req.Method, req.Host, req.URL.String())
	}

	switch {
	case strings.HasPrefix(req.RequestURI, "/debug/"):
		s.routerPprof.ServeHTTP(rw, req)
		return
	case strings.HasPrefix(req.RequestURI, "/custom_api/"):
		s.routerCustomAPI.ServeHTTP(rw, req)
		return
	}

	hosts := strings.Split(req.Host, ":")
	if len(hosts) != 2 {
		s.httpError(ctx, rw, fmt.Sprintf("invalid host: %s", req.Host), http.StatusBadRequest)
		return
	}
	var proxyHost string
	var proxyType options.ProxyType
	var requestURI = req.RequestURI
	switch hosts[0] {
	// 如果传递过来的 Host 是本地地址，则认为用户使用的是 RegistryMirror 模式
	case LocalHost, LocalHostAddr:
		proxyType = options.RegistryMirror

		queryNS := req.URL.Query().Get("ns")
		if queryNS != "" {
			// for containerd
			proxyHost = queryNS
		} else {
			match := proxyHostRegex.FindStringSubmatch(req.RequestURI)
			// 如果 match[1] 未从 options 中查找到对应配置，则认为其 proxyHost 就是空的
			if len(match) > 1 && s.op.FilterRegistryMapping(match[1], proxyType) != nil {
				proxyHost = match[1]
				requestURI = strings.Replace(req.RequestURI, "/"+proxyHost+"/", "/", 1)
			}
		}
	// 传递过来的 Host 是个域名地址，则认为用户使用的是域名代理模式
	default:
		proxyType = options.DomainProxy
		proxyHost = hosts[0]
	}
	// logctx.Infof(ctx, "parse request, proxyType: %s, proxyHost: %s", string(proxyType), proxyHost)

	upstreamProxy := proxy.NewUpstreamProxy(proxyHost, proxyType, s.torrentHandler)
	if upstreamProxy == nil {
		s.httpError(ctx, rw, fmt.Sprintf("no handler for proxy host '%s'", proxyHost), http.StatusBadRequest)
		return
	}
	upstreamProxy.ServeHTTP(requestURI, rw, req)
}
