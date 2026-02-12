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
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/bittorrent"
	"github.com/penglongli/accelerboat/pkg/cleaner"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/metrics"
	"github.com/penglongli/accelerboat/pkg/ociscan"
	"github.com/penglongli/accelerboat/pkg/recorder"
	"github.com/penglongli/accelerboat/pkg/server/common"
	"github.com/penglongli/accelerboat/pkg/server/customapi"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/server/middleware"
	"github.com/penglongli/accelerboat/pkg/server/registry"
	"github.com/penglongli/accelerboat/pkg/staticwatcher"
)

// AccelerboatServer defines the accelerboat server
type AccelerboatServer struct {
	op        *options.AccelerBoatOption
	opWatcher options.OptionChangeWatcher

	globalCtx    context.Context
	globalCancel context.CancelFunc

	ginSvr      *gin.Engine
	httpServer  *http.Server
	httpSServer *http.Server
	ociScanner  *ociscan.ScanHandler

	torrentHandler *bittorrent.TorrentHandler
	staticWatcher  *staticwatcher.StaticFilesWatcher
}

// NewAccelerboatServer create the instance of Accelerboat
func NewAccelerboatServer(globalCtx context.Context, op *options.AccelerBoatOption,
	opWatcher options.OptionChangeWatcher) *AccelerboatServer {
	ctx, cancel := context.WithCancel(globalCtx)
	return &AccelerboatServer{
		op:           op,
		opWatcher:    opWatcher,
		globalCtx:    ctx,
		globalCancel: cancel,
	}
}

func (s *AccelerboatServer) Init() error {
	s.torrentHandler = bittorrent.NewTorrentHandler()
	if err := s.torrentHandler.Init(); err != nil {
		return err
	}
	s.ociScanner = ociscan.NewScanHandler()
	if err := s.ociScanner.Init(); err != nil {
		return err
	}
	logger.Infof("oci scanner init completed")
	s.staticWatcher = staticwatcher.NewStaticFileWatcher()
	if err := s.staticWatcher.Init(s.globalCtx); err != nil {
		return err
	}
	if s.op.StorageConfig.EventFile != "" {
		if err := recorder.Global.InitEventFile(s.op.StorageConfig.EventFile, recorder.DefaultEventFileMaxSizeMB,
			recorder.DefaultEventFileMaxBackups); err != nil {
			return err
		}
		logger.Infof("event file sink enabled: %s (rotate at 1GB, keep %d backups)", s.op.StorageConfig.EventFile,
			recorder.DefaultEventFileMaxBackups)
	}
	s.initHTTPRouter()
	return nil
}

func (s *AccelerboatServer) initHTTPRouter() {
	ginSvr := gin.New()
	ginSvr.Use(gin.Recovery())
	ginSvr.UseRawPath = true
	gin.SetMode(gin.ReleaseMode)
	ginSvr.Use(middleware.GinMiddleware())
	pprof.Register(ginSvr)
	ginSvr.GET("/metrics", gin.WrapH(promhttp.Handler()))
	ch := customapi.NewCustomHandler(s.op, s.torrentHandler, s.ociScanner)
	ch.Register(ginSvr)
	s.ginSvr = ginSvr
}

func (s *AccelerboatServer) Run() error {
	fs := []func(errCh chan error){s.runHTTPServer, s.runHTTPSServer, s.runOCITickReporter,
		s.runStaticFilesWatcher, s.runOptionFileWatcher, s.runDiskUsageUpdater}
	errCh := make(chan error, len(fs))
	for i := range fs {
		go fs[i](errCh)
	}
	imageCleaner := cleaner.NewImageCleaner(s.op)
	if err := imageCleaner.Init(); err != nil {
		return errors.Wrapf(err, "failed to init image cleaner")
	}
	go func() {
		<-s.globalCtx.Done()
		s.httpServer.Shutdown(context.Background())
		s.httpSServer.Shutdown(context.Background())
	}()
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
		Handler: s,
	}
	logger.Infof("http server listening on %s", serverAddr)
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
		logger.Infof("load tls cert, host: %s, original: %s", mp.ProxyHost, mp.OriginalHost)
		tlsCerts = append(tlsCerts, kp)
	}
	s.httpSServer = &http.Server{
		Addr:    serverAddr,
		Handler: s,
		TLSConfig: &tls.Config{
			Certificates: tlsCerts,
		},
	}
	logger.Infof("http(s) server listening on %s", serverAddr)
	if err = s.httpSServer.ListenAndServeTLS("", ""); err != nil &&
		!syserrors.Is(err, http.ErrServerClosed) {
		errCh <- err
		logger.Errorf("failed to start http(s) server: %s", err.Error())
		return
	}
	errCh <- nil
}

func (s *AccelerboatServer) runOCITickReporter(errCh chan error) {
	defer logger.Warnf("oci tick reporter exit")
	logger.Infof("oci reporter started")
	s.ociScanner.TickerReport(s.globalCtx)
	errCh <- nil
}

func (s *AccelerboatServer) runStaticFilesWatcher(errCh chan error) {
	defer logger.Warnf("static-files watcher exit")
	logger.Infof("static-files watcher started")
	if err := s.staticWatcher.Watch(s.globalCtx); err != nil {
		logger.Errorf("static-files watcher exit with err: %s", err.Error())
		errCh <- err
		return
	}
	errCh <- nil
}

func (s *AccelerboatServer) runOptionFileWatcher(errCh chan error) {
	defer logger.Warnf("option watcher exit")
	ch := s.opWatcher.Watch(s.globalCtx)
	logger.Infof("option watcher started")
L:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break L
			}
			// TODO: need do something but now do nothing
		}
	}
	errCh <- nil
}

func (s *AccelerboatServer) runDiskUsageUpdater(errCh chan error) {
	defer logger.Warnf("disk usage updater exit")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	paths := map[string]string{
		"transfer":  s.op.StorageConfig.TransferPath,
		"download":  s.op.StorageConfig.DownloadPath,
		"smallfile": s.op.StorageConfig.SmallFilePath,
		"torrent":   s.op.StorageConfig.TorrentPath,
		"oci":       s.op.StorageConfig.OCIPath,
	}
	metrics.UpdateDiskUsage(paths)
	for {
		select {
		case <-s.globalCtx.Done():
			errCh <- nil
			return
		case <-ticker.C:
			metrics.UpdateDiskUsage(paths)
		}
	}
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
	rec := common.NewResponseRecorder(rw)
	start := time.Now()
	method := req.Method

	for _, v := range s.ginSvr.Routes() {
		if req.URL.Path == v.Path && req.Method == v.Method {
			s.ginSvr.ServeHTTP(rec, req)
			path := v.Path
			if path == "" {
				path = req.URL.Path
			}
			if _, ok := apitypes.NotPrintLog[path]; ok {
				return
			}
			metrics.HTTPRequestsTotal.WithLabelValues("localhost", method, path, strconv.Itoa(rec.Status())).Inc()
			metrics.HTTPRequestDurationSeconds.WithLabelValues("localhost", method, path).
				Observe(time.Since(start).Seconds())
			return
		}
	}

	req = middleware.GeneralMiddleware(rec, req)
	ctx := req.Context()
	hosts := strings.Split(req.Host, ":")
	if len(hosts) != 2 {
		s.httpError(ctx, rec, fmt.Sprintf("invalid host: %s", req.Host), http.StatusBadRequest)
		return
	}
	proxyType := options.DomainProxy
	proxyHost := hosts[0]
	requestURI := req.RequestURI

	upstreamProxy := registry.NewUpstreamProxy(proxyType, proxyHost, s.torrentHandler)
	if upstreamProxy == nil {
		s.httpError(ctx, rec, fmt.Sprintf("no handler for proxy host '%s'", proxyHost), http.StatusBadRequest)
		return
	}
	upstreamProxy.ServeHTTP(requestURI, rec, req)
	metrics.HTTPRequestsTotal.WithLabelValues(proxyHost, method, "", strconv.Itoa(rec.Status())).Inc()
	if !strings.Contains(req.URL.Path, "/blobs/") {
		metrics.HTTPRequestDurationSeconds.WithLabelValues(proxyHost, method, proxyHost).
			Observe(time.Since(start).Seconds())
	}
}
