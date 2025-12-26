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
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/bittorrent"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/customapi"
	"github.com/penglongli/accelerboat/pkg/server/middleware"
	"github.com/penglongli/accelerboat/pkg/server/registry"
)

type AccelerboatServer struct {
	op *options.AccelerBoatOption

	ginSvr      *gin.Engine
	httpServer  *http.Server
	httpSServer *http.Server

	torrentHandler *bittorrent.TorrentHandler
}

func NewAccelerboatServer(op *options.AccelerBoatOption) *AccelerboatServer {
	return &AccelerboatServer{
		op: op,
	}
}

func (s *AccelerboatServer) Init() error {
	s.torrentHandler = bittorrent.NewTorrentHandler()
	if err := s.torrentHandler.Init(); err != nil {
		return err
	}
	s.initHTTPRouter()
	return nil
}

func (s *AccelerboatServer) initHTTPRouter() {
	ginSvr := gin.New()
	ginSvr.Use(gin.Recovery())
	ginSvr.UseRawPath = true
	gin.SetMode(gin.ReleaseMode)
	ginSvr.Use(middleware.CommonMiddleware())
	pprof.Register(ginSvr)
	ch := customapi.NewCustomHandler(s.op)
	ch.Register(ginSvr)
	ginSvr.NoRoute(func(c *gin.Context) {
		s.ServeHTTP(c.Writer, c.Request)
	})
	s.ginSvr = ginSvr
}

func (s *AccelerboatServer) Run() error {
	fs := []func(errCh chan error){s.runHTTPServer, s.runHTTPSServer}
	errCh := make(chan error, len(fs))
	for i := range fs {
		go fs[i](errCh)
	}
	// for-loop wait every goroutine normal finish
	for i := 0; i < len(fs); i++ {
		e := <-errCh
		// we should return error if e not nil, perhaps some goroutines are
		// exited with error. So we need exit the server
		if e != nil {
			return errors.Wrapf(e, "run server failed")
		}
	}
	return nil
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

const (
	// LocalHost defines the localhost
	LocalHost = "localhost"
	// LocalHostAddr defines the localhost address
	LocalHostAddr = "127.0.0.1"
)

func (s *AccelerboatServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	hosts := strings.Split(req.Host, ":")
	if len(hosts) != 2 {
		s.httpError(ctx, rw, fmt.Sprintf("invalid host: %s", req.Host), http.StatusBadRequest)
		return
	}
	proxyType := options.DomainProxy
	proxyHost := hosts[0]
	requestURI := req.RequestURI

	upstreamProxy := registry.NewUpstreamProxy(proxyType, proxyHost, s.torrentHandler)
	if upstreamProxy == nil {
		s.httpError(ctx, rw, fmt.Sprintf("no handler for proxy host '%s'", proxyHost), http.StatusBadRequest)
		return
	}
	upstreamProxy.ServeHTTP(requestURI, rw, req)
}
