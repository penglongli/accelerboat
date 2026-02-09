// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/bittorrent"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/metrics"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/server/customapi/requester"
	"github.com/penglongli/accelerboat/pkg/store"
	"github.com/penglongli/accelerboat/pkg/utils"
	"github.com/penglongli/accelerboat/pkg/utils/formatutils"
	"github.com/penglongli/accelerboat/pkg/utils/httpfile"
	"github.com/penglongli/accelerboat/pkg/utils/lock"
)

// UpstreamProxyInterface defines the interface of upstream
type UpstreamProxyInterface interface {
	ServeHTTP(requestURI string, rw http.ResponseWriter, req *http.Request)
}

type upstreamProxy struct {
	op            *options.AccelerBoatOption
	proxyHost     string
	originalHost  string
	proxyType     options.ProxyType
	proxyRegistry *options.RegistryMapping
	reverseProxy  *httputil.ReverseProxy

	layerLock lock.Interface

	cacheStore     store.CacheStore
	torrentHandler *bittorrent.TorrentHandler
}

var (
	createLock sync.Mutex
	proxies    = &sync.Map{}
)

func buildProxyKey(proxyType options.ProxyType, proxyHost string) string {
	return fmt.Sprintf("%s_%s", proxyType, proxyHost)
}

func NewUpstreamProxy(proxyType options.ProxyType, proxyHost string,
	torrentHandler *bittorrent.TorrentHandler) UpstreamProxyInterface {
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
		op:             op,
		proxyHost:      proxyHost,
		proxyType:      proxyType,
		originalHost:   proxyRegistry.OriginalHost,
		proxyRegistry:  proxyRegistry,
		cacheStore:     store.GlobalRedisStore(),
		layerLock:      lock.NewLocalLock(),
		torrentHandler: torrentHandler,
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
			metrics.RecordError(metrics.ComponentReverseProxy, "reverse_proxy")
			logger.ErrorContextf(req.Context(), "reverse proxy to '%s, %s' failed: %s (req-headers: %+v)",
				req.Method, req.URL.String(), err.Error(), req.Header)
		},
		Transport: p.op.HTTPProxyTransport(),
		ModifyResponse: func(resp *http.Response) error {
			req := resp.Request
			logger.InfoContextf(req.Context(), "reverse proxy to '%s, %s' response code '%d'",
				req.Method, req.URL.String(), resp.StatusCode)
			utils.ChangeAuthenticateHeader(resp, fmt.Sprintf("https://%s:%d", p.proxyRegistry.ProxyHost,
				p.op.HTTPSPort))
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
	originalHost := p.originalHost
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

	// directly reverse if registry-mapping is disabled
	proxyRegistry := p.op.FilterRegistryMapping(p.proxyHost, p.proxyType)
	if proxyRegistry != nil && !proxyRegistry.Enable {
		p.reverseProxy.ServeHTTP(rw, req)
		return
	}

	registryService, registryScope, isServiceToken := utils.IsServiceToken(req)
	headManifestRepo, headManifestTag, isHeadManifest := utils.IsHeadImageDigest(req)
	manifestRepo, manifestTag, isGetManifest := utils.IsManifestGet(req)
	blobRepo, digest, isGetBlob := utils.IsBlobGet(req.URL.Path)
	switch {
	case isServiceToken:
		if registryService == "" || registryScope == "" {
			break
		}
		ctx = logger.WithContextFields(ctx, "service", registryService, "scope", registryScope)
		err = p.handleGetServiceToken(ctx, req, rw, registryService, registryScope)
		if err == nil {
			return
		}
		logger.ErrorContextf(ctx, "service-token request failed and will reverse: %s", err.Error())
	case isHeadManifest:
		if auth := req.Header.Get("Authorization"); auth != "" {
			err = p.handleHeadManifest(ctx, req, rw, headManifestRepo, headManifestTag)
			if err == nil {
				return
			}
			logger.ErrorContextf(ctx, "head-manifest request failed and will reverse: %s", err.Error())
		}
	case isGetManifest:
		ctx = logger.WithContextFields(ctx, "repo", manifestRepo, "tag", manifestTag)
		err = p.handleGetManifest(ctx, req, rw, manifestRepo, manifestTag)
		if err == nil {
			return
		}
		logger.ErrorContextf(ctx, "get-manifest request failed and will reverse: %s", err.Error())
	case isGetBlob:
		ctx = logger.WithContextFields(ctx, "repo", blobRepo, "digest", digest)
		if err = p.handleGetBlob(ctx, req, rw, blobRepo, digest); err == nil {
			return
		}
		logger.ErrorContextf(ctx, "get-blob request failed: %s", err.Error())
	}
	req = req.WithContext(ctx)
	p.recorderReverseProxy(req)
	p.reverseProxy.ServeHTTP(rw, req)
}

func (p *upstreamProxy) handleGetServiceToken(ctx context.Context, req *http.Request, rw http.ResponseWriter,
	service, scope string) error {
	start := time.Now()
	logger.InfoContextf(ctx, "handle service-token request")
	getServiceTokenReq := &apitypes.GetServiceTokenRequest{
		OriginalHost:    p.originalHost,
		ServiceTokenUrl: req.URL.String(),
		Headers:         req.Header,
		Service:         service,
		Scope:           scope,
	}
	master, serviceToken, err := requester.GetServiceToken(ctx, getServiceTokenReq)
	p.recorderServiceToken(start, master, service, scope, err)
	if err != nil {
		return err
	}
	logger.InfoContextf(ctx, "get service-token from master(%s) success", master)
	logger.V(3).InfoContextf(ctx, "service token: %s", serviceToken)
	rw.WriteHeader(http.StatusOK)
	rw.Header().Add("Content-Type", "application/json")
	_, _ = rw.Write([]byte(serviceToken))
	return nil
}

func (p *upstreamProxy) handleHeadManifest(ctx context.Context, req *http.Request, rw http.ResponseWriter,
	repo, tag string) error {
	start := time.Now()
	ctx = logger.WithContextFields(ctx, "repo", repo, "tag", tag)
	logger.InfoContextf(ctx, "handle head-manifest request")
	headManifestReq := &apitypes.HeadManifestRequest{
		OriginalHost:    req.Host,
		HeadManifestUrl: req.URL.RequestURI(),
		Headers:         req.Header,
		Repo:            repo,
		Tag:             tag,
	}
	master, respHeaders, err := requester.HeadManifest(ctx, headManifestReq)
	p.recorderHeadManifest(start, master, repo, tag, err)
	if err != nil {
		return err
	}
	for k, v := range respHeaders {
		for _, vv := range v {
			rw.Header().Add(k, vv)
		}
	}
	logger.InfoContextf(ctx, "head-manifest from master(%s) success", master)
	rw.WriteHeader(http.StatusOK)
	return nil
}

func (p *upstreamProxy) handleGetManifest(ctx context.Context, req *http.Request, rw http.ResponseWriter,
	repo, tag string) error {
	start := time.Now()
	logger.InfoContextf(ctx, "handle get-manifest request")
	getManifestReq := &apitypes.GetManifestRequest{
		OriginalHost: req.Host,
		ManifestUrl:  req.URL.RequestURI(),
		Headers:      req.Header,
		Repo:         repo,
		Tag:          tag,
	}
	master, manifest, err := requester.GetManifest(ctx, getManifestReq)
	p.recorderGetManifest(start, master, repo, tag, manifest, err)
	if err != nil {
		return err
	}
	logger.InfoContextf(ctx, "get manifest from master(%s) success", master)
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(manifest))
	return nil
}

func (p *upstreamProxy) handleGetBlob(ctx context.Context, req *http.Request, rw http.ResponseWriter,
	repo, digest string) error {
	logger.InfoContextf(ctx, "handle get-blob request")
	p.layerLock.Lock(ctx, digest)
	// directly download if check layer existed in-local
	lfi, lp := p.checkLocalLayer(digest)
	if lfi != nil {
		start := time.Now()
		p.layerLock.UnLock(ctx, digest)
		if p.downloadLayerFromLocalLimit(ctx, digest, req, rw) {
			p.recorderServeBlobFromLocal(start, repo, digest, lfi.Size(), nil)
			return nil
		}
		p.recorderServeBlobFromLocal(start, repo, digest, lfi.Size(),
			fmt.Errorf("serve local file '%s' not success", lp))
		return fmt.Errorf("download from local '%s' not success(local exist)", lp)
	}
	defer p.layerLock.UnLock(ctx, digest)

	logger.InfoContextf(ctx, "start get layer-info from master")
	layerReq := &apitypes.DownloadLayerRequest{
		OriginalHost: req.Host,
		LayerUrl:     req.URL.RequestURI(),
		Headers:      req.Header,
		Repo:         repo,
		Digest:       digest,
	}
	start := time.Now()
	layerResp, master, err := requester.DownloadLayerFromMaster(ctx, layerReq, digest)
	p.recorderGetBlobFromMaster(start, master, repo, digest, layerResp, err)
	if err != nil {
		return errors.Wrapf(err, "download layer from master failed, master=%s", master)
	}
	haveTorrent := "no-torrent"
	if layerResp.TorrentBase64 != "" {
		haveTorrent = "(too long not print)"
	}

	logger.InfoContextf(ctx, "get layer-info from master(%s) success, located: %s, "+
		"filePath: %s, size: %s, torrent: %s", master, layerResp.Located, layerResp.FilePath,
		formatutils.FormatSize(layerResp.FileSize), haveTorrent)
	// Should download layer from local again, maybe already have it on local
	// Because when we download the layer from the master, the master may assign the task of downloading the
	// layer to us. When we get the layer information, the layer may have been downloaded to the current node.
	if p.downloadLayerFromLocalLimit(ctx, digest, req, rw) {
		p.recorderServeBlobFromLocal(start, repo, digest, layerResp.FileSize, nil)
		return nil
	}

	// Download layer from remote to localhost
	if err = p.handleLayerDownload(ctx, layerResp, repo, digest); err != nil {
		return errors.Wrapf(err, "handle download layer failed")
	}
	// Serve blob layer from local to client(docker/containerd)
	if p.downloadLayerFromLocalLimit(ctx, digest, req, rw) {
		p.recorderServeBlobFromLocal(start, repo, digest, layerResp.FileSize, nil)
		return nil
	}
	p.recorderServeBlobFromLocal(start, repo, digest, layerResp.FileSize,
		fmt.Errorf("download from local not success"))
	return fmt.Errorf("download layer from local not success(after download)")
}

func (p *upstreamProxy) checkLocalLayer(digest string) (os.FileInfo, string) {
	layerName := utils.LayerFileName(digest)
	localLayer := path.Join(p.op.StorageConfig.TransferPath, layerName)
	fi, err := os.Stat(localLayer)
	if err == nil {
		return fi, localLayer
	}
	localLayer = path.Join(p.op.StorageConfig.SmallFilePath, layerName)
	fi, err = os.Stat(localLayer)
	if err == nil {
		return fi, localLayer
	}
	localLayer = path.Join(p.op.StorageConfig.OCIPath, layerName)
	fi, err = os.Stat(localLayer)
	if err == nil {
		return fi, localLayer
	}
	return nil, ""
}

var downloadSem = make(chan struct{}, 20)

func (p *upstreamProxy) downloadLayerFromLocalLimit(ctx context.Context, digest string, req *http.Request,
	rw http.ResponseWriter) bool {
	logger.InfoContextf(ctx, "download layer from local waiting limit lock")
	select {
	case downloadSem <- struct{}{}:
		defer func() { <-downloadSem }()
	}
	return p.downloadLayerFromLocal(ctx, digest, req, rw)
}

// downloadLayerFromLocal download layer from local, if local have the layer
func (p *upstreamProxy) downloadLayerFromLocal(ctx context.Context, digest string, req *http.Request,
	rw http.ResponseWriter) bool {
	layerFileInfo, layerPath := p.checkLocalLayer(digest)
	if layerFileInfo == nil {
		logger.WarnContextf(ctx, "not found digest '%s' in local", digest)
		return false
	}
	logger.InfoContextf(ctx, "download layer from local starting")
	if err := httpfile.HTTPServeFile(ctx, rw, req, layerPath); err != nil {
		logger.WarnContextf(ctx, "download layer from local failed with error: %s", err.Error())
		return false
	}
	logger.InfoContextf(ctx, "download layer from local success. Content-Length: %d", layerFileInfo.Size())
	return true
}

func (p *upstreamProxy) handleLayerDownload(ctx context.Context, resp *apitypes.DownloadLayerResponse,
	repo, digest string) error {
	start := time.Now()
	// download layer from target directly with tcp
	if resp.TorrentBase64 == "" {
		err := p.downloadByTCP(ctx, resp.Located, resp.FilePath, digest)
		p.recorderDownloadBlobByTCP(start, repo, digest, resp, err)
		if err != nil {
			return errors.Wrapf(err, "download by tcp failed")
		}
		return nil
	}

	err := p.torrentHandler.DownloadTorrent(ctx, digest, resp.TorrentBase64, resp.FilePath)
	p.recorderDownloadBlobByTorrent(start, repo, digest, resp, err)
	if err == nil {
		return nil
	} else {
		logger.WarnContextf(ctx, "downlaod layer with torrent failed and will download-by-tcp: %s",
			err.Error())
	}

	err = p.downloadByTCP(ctx, resp.Located, resp.FilePath, digest)
	p.recorderDownloadBlobByTCP(start, repo, digest, resp, err)
	if err != nil {
		return errors.Wrapf(err, "download by tcp failed")
	}
	return nil
}

func (p *upstreamProxy) downloadByTCP(ctx context.Context, target string, filePath, digest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:%d%s", target,
		p.op.HTTPPort, apitypes.APITransferLayerTCP), nil)
	if err != nil {
		return errors.Wrapf(err, "create http.request failed")
	}
	query := req.URL.Query()
	query.Set("file", filePath)
	req.URL.RawQuery = query.Encode()
	logger.InfoContextf(ctx, "download layer from target '%s' with tcp starting", target)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrapf(err, "download layer from target '%s' with tcp failed", target)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("download layer from target '%s' with tcp resp code not 200 but %d",
			target, resp.StatusCode)
	}
	if err = p.saveLayerToLocal(ctx, resp, digest, filePath); err != nil {
		return errors.Wrapf(err, "download to local failed")
	}
	return nil
}

func (p *upstreamProxy) saveLayerToLocal(ctx context.Context, resp *http.Response,
	digest, newFile string) error {
	tmpFile := path.Join(p.op.StorageConfig.DownloadPath, digest+".tar.gzip")
	out, err := os.Create(tmpFile)
	if err != nil {
		return errors.Wrapf(err, "create file %s failed", tmpFile)
	}
	defer out.Close()
	if _, err = io.Copy(out, resp.Body); err != nil {
		return errors.Wrapf(err, "download-by-tcp io.copy failed")
	}
	logger.InfoContextf(ctx, "layer download to local '%s' success", tmpFile)
	if err = os.Rename(tmpFile, newFile); err != nil {
		return errors.Wrapf(err, "rename file %s to %s failed", tmpFile, newFile)
	}
	logger.InfoContextf(ctx, "rename file %s to %s success", tmpFile, newFile)
	return nil
}
