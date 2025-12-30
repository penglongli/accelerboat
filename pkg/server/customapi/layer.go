// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/common"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/server/customapi/requester"
	"github.com/penglongli/accelerboat/pkg/utils"
	"github.com/penglongli/accelerboat/pkg/utils/formatutils"
	"github.com/penglongli/accelerboat/pkg/utils/httputils"
)

func buildContentLengthKey(host, digest string) string {
	return fmt.Sprintf("%s,%s", host, digest)
}

func (h *CustomHandler) getLayerContentLength(ctx context.Context, req *apitypes.DownloadLayerRequest) (int64, error) {
	lockKey := buildContentLengthKey(req.OriginalHost, req.Digest)
	h.layerContentLengthLock.Lock(ctx, lockKey)
	defer h.layerContentLengthLock.UnLock(ctx, lockKey)

	v := h.layerContentLengths.Get(lockKey)
	if v != nil && !v.IsExpired() {
		return v.Value(), nil
	}
	logger.InfoContextf(ctx, "handling get layer content-length")
	resp, _, err := httputils.SendHTTPRequestReturnResponse(ctx, &httputils.HTTPRequest{
		Url:         fmt.Sprintf("https://%s%s", req.OriginalHost, req.LayerUrl),
		Method:      http.MethodHead,
		HeaderMulti: req.Headers,
	})
	if err != nil {
		return 0, errors.Wrapf(err, "get layer content-length failed")
	}
	h.layerContentLengths.Set(lockKey, resp.ContentLength, 30*time.Second)
	layerSize := formatutils.FormatSize(resp.ContentLength)
	logger.InfoContextf(ctx, "get layer content-length success: %s(%d)", layerSize, resp.ContentLength)
	return resp.ContentLength, nil
}

// GetLayerInfo master will handle the GetLayerInfo request.
// It will check layer has cached in all the cluster, or will distribute download request to other
// node.
func (h *CustomHandler) GetLayerInfo(c *gin.Context) (interface{}, error) {
	req := &apitypes.DownloadLayerRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	ctx := c.Request.Context()
	contentLength, err := h.getLayerContentLength(ctx, req)
	if err != nil {
		return nil, err
	}

	h.downloadLayerLock.Lock(ctx, req.Digest)
	defer h.downloadLayerLock.UnLock(ctx, req.Digest)
	resp, err := h.checkLayerHasCached(ctx, req, contentLength)
	if err == nil {
		return resp, nil
	}

	logger.WarnContextf(ctx, "check layer has cached failed: %s", err.Error())
	// master should download directly if small layer
	if contentLength < common.TwentyMB {
		resultPath := path.Join(h.op.StorageConfig.SmallFilePath, utils.LayerFileName(req.Digest))
		if err = h.requestDownloadLayer(ctx, req, resultPath); err != nil {
			return nil, fmt.Errorf("download small-layer from original registry '%s/%s' failed",
				req.OriginalHost, req.LayerUrl)
		}
		return &apitypes.DownloadLayerResponse{
			Located:  h.op.Address,
			FilePath: resultPath,
			FileSize: contentLength,
		}, nil
	}
	// distribute the layer download task to other nodes.
	if resp, err = h.distributeDownloadLayer(ctx, req); err != nil {
		return nil, err
	}
	return resp, nil
}

func (h *CustomHandler) checkLayerHasCached(ctx context.Context, req *apitypes.DownloadLayerRequest,
	contentLength int64) (*apitypes.DownloadLayerResponse, error) {
	staticLayers, ociLayers, err := h.cacheStore.QueryLayers(ctx, req.Digest)
	if err != nil {
		return nil, errors.Wrapf(err, "query layers from cache store failed")
	}
	for _, sl := range staticLayers {
		logger.InfoContextf(ctx, "check static layer '%s, %s' starting", sl.Located, sl.Data)
		var resp *apitypes.CheckStaticLayerResponse
		if resp, err = requester.CheckStaticLayer(ctx, sl.Located, &apitypes.CheckStaticLayerRequest{
			OriginalHost:          req.OriginalHost,
			Repo:                  req.Repo,
			Digest:                req.Digest,
			LayerPath:             sl.Data,
			ExpectedContentLength: contentLength,
		}); err != nil {
			logger.ErrorContextf(ctx, "check static layer '%s, %s' failed: %s",
				sl.Located, sl.Data, err.Error())
			continue
		}
		logger.InfoContextf(ctx, "check static layer '%s, %s' success", sl.Located, sl.Data)
		return &apitypes.DownloadLayerResponse{
			TorrentBase64: resp.TorrentBase64,
			Located:       resp.Located,
			FileSize:      resp.FileSize,
			FilePath:      resp.LayerPath,
		}, nil
	}
	for _, ocil := range ociLayers {
		logger.InfoContextf(ctx, "check oci-layer '%s, %s' starting'", ocil.Located, ocil.Data)
		var resp *apitypes.CheckOCILayerResponse
		if resp, err = requester.CheckOCILayer(ctx, ocil.Located, &apitypes.CheckOCILayerRequest{
			Digest:  req.Digest,
			OCIType: string(ocil.Type),
		}); err != nil {
			logger.ErrorContextf(ctx, "check oci-layer '%s, %s' failed: %s",
				ocil.Located, ocil.Data, err.Error())
			continue
		}
		return &apitypes.DownloadLayerResponse{
			TorrentBase64: resp.TorrentBase64,
			Located:       resp.Located,
			FileSize:      resp.FileSize,
			FilePath:      resp.LayerPath,
		}, nil
	}
	return nil, fmt.Errorf("not found cached layer, checked static[%d] oci[%d]",
		len(staticLayers), len(ociLayers))
}

func (h *CustomHandler) distributeDownloadLayer(ctx context.Context, req *apitypes.DownloadLayerRequest) (
	*apitypes.DownloadLayerResponse, error) {
	var resp *apitypes.DownloadLayerResponse
	var err error
	for i := 0; i < 5; i++ {
		targetNode := h.distributeNode()
		logger.InfoContextf(ctx, "distribute task to node '%s'", targetNode)
		if resp, err = requester.DownloadLayerFromNode(ctx, targetNode, req); err != nil {
			logger.ErrorContextf(ctx, "node '%s' download layer failed: %s", targetNode, err.Error())
		} else {
			logger.InfoContextf(ctx, "node '%s' download layer success", targetNode)
		}
		h.releaseNode(targetNode)
		if err == nil {
			return resp, nil
		}
	}
	return nil, errors.Wrapf(err, "distribute download layer failed")
}

func (h *CustomHandler) distributeNode() string {
	h.nodeDownloadLock.Lock()
	defer h.nodeDownloadLock.Unlock()

	if h.nodeDownloadTasks == nil {
		h.nodeDownloadTasks = make(map[string]int)
	}
	eps := h.op.ServiceDiscovery.Endpoints
	epMap := make(map[string]struct{})
	for _, ep := range eps {
		epMap[ep] = struct{}{}
		_, ok := h.nodeDownloadTasks[ep]
		if !ok {
			h.nodeDownloadTasks[ep] = 0
		}
	}
	for k := range epMap {
		if _, ok := h.nodeDownloadTasks[k]; !ok {
			delete(h.nodeDownloadTasks, k)
		}
	}
	var result string
	ans := 100000
	for k, v := range h.nodeDownloadTasks {
		if ans > v {
			ans = v
			result = k
		}
	}
	h.nodeDownloadTasks[result]++
	return result
}

func (h *CustomHandler) releaseNode(node string) {
	h.nodeDownloadLock.Lock()
	defer h.nodeDownloadLock.Unlock()
	if v, ok := h.nodeDownloadTasks[node]; ok {
		h.nodeDownloadTasks[node] = v - 1
	}
}

func (h *CustomHandler) DownloadLayer(c *gin.Context) (interface{}, error) {
	req := &apitypes.DownloadLayerRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	resultPath := path.Join(h.op.StorageConfig.TransferPath, utils.LayerFileName(req.Digest))
	ctx := c.Request.Context()
	if err := h.requestDownloadLayer(ctx, req, resultPath); err != nil {
		return nil, errors.Wrapf(err, "download layer failed")
	}
	fileSize, err := checkLocalLayer(resultPath)
	if err != nil {
		return nil, errors.Wrapf(err, "check local layer failed")
	}
	resp := &apitypes.DownloadLayerResponse{
		Located:  h.op.Address,
		FilePath: resultPath,
		FileSize: fileSize,
	}

	if !h.op.TorrentConfig.Enable || fileSize < h.op.TorrentConfig.Threshold {
		return resp, nil
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	torrentBase64, err := h.torrentHandler.GenerateTorrent(timeoutCtx, req.Digest, resultPath)
	if err != nil {
		logger.ErrorContextf(ctx, "generate torrent for '%s' failed: %s", resultPath, err.Error())
	} else {
		resp.TorrentBase64 = torrentBase64
	}
	return resp, nil
}

// requestDownloadLayer request the original registry to download layer
func (h *CustomHandler) requestDownloadLayer(ctx context.Context, req *apitypes.DownloadLayerRequest,
	destPath string) error {
	logger.InfoContextf(ctx, "starting download layer from original registry")
	resp, err := httputils.SendHTTPRequestOnlyResponse(ctx, &httputils.HTTPRequest{
		Url:         fmt.Sprintf("https://%s%s", req.OriginalHost, req.LayerUrl),
		Method:      http.MethodGet,
		HeaderMulti: req.Headers,
	})
	if err != nil {
		return errors.Wrapf(err, "download layer from original registry failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var bs []byte
		bs, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("download layer from original registry failed, statusCode: %d", resp.StatusCode)
		}
		return fmt.Errorf("download layer from original registry failed, statusCode=%d: %s",
			resp.StatusCode, string(bs))
	}

	contentLength := resp.ContentLength
	layerSize := formatutils.FormatSize(contentLength)

	layerFullPath := path.Join(h.op.StorageConfig.DownloadPath, utils.LayerFileName(req.Digest))
	_ = os.RemoveAll(layerFullPath)
	layer, err := os.Create(layerFullPath)
	if err != nil {
		return errors.Wrapf(err, "create layer file '%s' failed", layerFullPath)
	}
	defer layer.Close()

	progressCh := make(chan struct{})
	go func() {
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				var fi os.FileInfo
				if fi, err = layer.Stat(); err != nil {
					logger.InfoContextf(ctx, "downloading layer from original registry '%s' got stats failed: %s",
						layerFullPath, err.Error())
				} else {
					percent := float64(fi.Size()) / float64(resp.ContentLength) * 100
					downloadSize := formatutils.FormatSize(fi.Size())
					logger.InfoContextf(ctx, "downloading layer from original registry(%.2f%%): %s/%s",
						percent, downloadSize, layerSize)
				}
			case <-progressCh:
				return
			}
		}
	}()
	defer close(progressCh)
	if _, err = io.Copy(layer, resp.Body); err != nil {
		_ = os.RemoveAll(layer.Name())
		return errors.Wrapf(err, "handle download_layer io copy failed")
	}
	logger.InfoContextf(ctx, "download layer '%s' successfully", layerFullPath)
	if err = os.Rename(layerFullPath, destPath); err != nil {
		return errors.Wrapf(err, "renamse '%s' to '%s' failed", layerFullPath, destPath)
	}
	return nil
}
