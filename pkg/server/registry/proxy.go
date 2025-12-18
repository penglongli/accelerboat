// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/store"
	"github.com/penglongli/accelerboat/pkg/utils"
)

// UpstreamProxyInterface defines the interface of upstream
type UpstreamProxyInterface interface {
	ServeHTTP(requestURI string, rw http.ResponseWriter, req *http.Request)
}

type upstreamProxy struct {
	op            *options.AccelerBoatOption
	proxyHost     string
	proxyType     options.ProxyType
	proxyRegistry *options.RegistryMapping
	reverseProxy  *httputil.ReverseProxy

	cacheStore store.CacheStore
}

var (
	createLock sync.Mutex
	proxies    = &sync.Map{}
)

func buildProxyKey(proxyType options.ProxyType, proxyHost string) string {
	return fmt.Sprintf("%s_%s", proxyType, proxyHost)
}

func NewUpstreamProxy(proxyType options.ProxyType, proxyHost string) UpstreamProxyInterface {
	pk := buildProxyKey(proxyType, proxyHost)
	v, ok := proxies.Load(pk)
	if ok {
		return v.(UpstreamProxyInterface)
	}

	createLock.Lock()
	defer createLock.Unlock()
	// try fetching again to avoid critical requests.
	v, ok = proxies.Load(pk)
	if ok {
		return v.(UpstreamProxyInterface)
	}
	op := options.GlobalOptions()
	proxyRegistry := op.FilterRegistryMapping(proxyHost, proxyType)
	if proxyRegistry == nil {
		return nil
	}
	p := &upstreamProxy{
		op:            op,
		proxyHost:     proxyHost,
		proxyType:     proxyType,
		proxyRegistry: op.FilterRegistryMapping(proxyHost, proxyType),
		cacheStore:    store.GlobalRedisStore(),
	}
	p.initReverseProxy()
	proxies.Store(pk, p)
	return p
}

// initReverseProxy will reverse the request to original registry host
func (p *upstreamProxy) initReverseProxy() {
	p.reverseProxy = &httputil.ReverseProxy{
		Director: func(request *http.Request) {},
		ErrorHandler: func(writer http.ResponseWriter, req *http.Request, err error) {
			logger.ErrorContextf(req.Context(), "reverse proxy '%s' failed: %s. header: %+v",
				req.URL.String(), err.Error(), req.Header)
		},
		// Transport: transport.DefaultProxyTransport(p.proxyRegistry),
		ModifyResponse: func(resp *http.Response) error {
			req := resp.Request
			logger.InfoContextf(req.Context(), "reverse proxy to '%s' response code '%d'",
				req.URL.String(), resp.StatusCode)
			return nil
		},
	}
}

func (p *upstreamProxy) httpError(ctx context.Context, rw http.ResponseWriter, errMsg string, code int) {
	logger.ErrorContextf(ctx, "upstream-proxy response error: %s", errMsg)
	http.Error(rw, errMsg, http.StatusBadRequest)
}

// ServeHTTP handle the request of upstream. Requests are divided into three categories: Auth/GetManifest/DownloadLayer.
// The function will handle the three requests.
func (p *upstreamProxy) ServeHTTP(requestURI string, rw http.ResponseWriter, req *http.Request) {
	originalHost := p.proxyRegistry.OriginalHost
	ctx := logger.WithContextFields(req.Context(), "registry", originalHost)
	fullPath := fmt.Sprintf("https://%s%s", originalHost, requestURI)
	newURL, err := url.Parse(fullPath)
	if err != nil {
		p.httpError(ctx, rw, fmt.Sprintf("build new full path '%s' failed: %s", fullPath, err.Error()),
			http.StatusBadRequest)
		return
	}
	req.URL = newURL
	req.Host = originalHost

	manifestRepo, manifestTag, isManifest := utils.IsManifestGet(req)
	blobRepo, digest, isBlob := utils.IsBlobGet(req.URL.Path)
	isHeadDigest := utils.IsHeadImageDigest(req)
	switch {
	case isManifest:
	case isBlob:

	case isHeadDigest:
	default:
		logger.InfoContextf(ctx, "handling request not manifest and blob")
	}
	// even though we intercept the request and something goes wrong,
	// we can still go back to the source registry
	if isManifest || isBlob || isHeadDigest {
	}
	req = req.WithContext(ctx)
	p.reverseProxy.ServeHTTP(rw, req)
}

func (p *upstreamProxy) handleManifestGetRequest() {

}
