// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jellydator/ttlcache/v3"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/bittorrent"
	"github.com/penglongli/accelerboat/pkg/ociscan"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/store"
	"github.com/penglongli/accelerboat/pkg/utils/lock"
)

// CustomHandler 定义了一系列的方法，用于对外提供服务. 通常是由普通 node 来调用 master 的这些对外接口能力.
type CustomHandler struct {
	op         *options.AccelerBoatOption
	cacheStore store.CacheStore

	authLock               lock.Interface
	authTokens             *ttlcache.Cache[string, *apitypes.RegistryAuthToken]
	headManifestLock       lock.Interface
	headManifests          *ttlcache.Cache[string, map[string][]string]
	getManifestLock        lock.Interface
	manifests              *ttlcache.Cache[string, string]
	layerContentLengthLock lock.Interface
	layerContentLengths    ttlcache.Cache[string, int64]
	downloadLayerLock      lock.Interface

	nodeDownloadLock  sync.Mutex
	nodeDownloadTasks map[string]int

	torrentHandler *bittorrent.TorrentHandler
	ociScanner     *ociscan.ScanHandler
}

func NewCustomHandler(op *options.AccelerBoatOption) *CustomHandler {
	return &CustomHandler{
		op: op,
	}
}

func (h *CustomHandler) Register(ginSvr *gin.Engine) {
	ginSvr.Handle(http.MethodPost, apitypes.APIGetServiceToken, h.HTTPWrapper(h.GetServiceToken))
	ginSvr.Handle(http.MethodPost, apitypes.APIHeadManifest, h.HTTPWrapper(h.RegistryHeadManifest))
	ginSvr.Handle(http.MethodPost, apitypes.APIGetManifest, h.HTTPWrapper(h.RegistryGetManifest))

	ginSvr.Handle(http.MethodGet, apitypes.APICheckStaticLayer, h.HTTPWrapper(h.CheckStaticLayer))
	ginSvr.Handle(http.MethodGet, apitypes.APICheckOCILayer, h.HTTPWrapper(h.CheckOCILayer))

	ginSvr.Handle(http.MethodPost, apitypes.APIGetLayerInfo, h.HTTPWrapper(h.GetLayerInfo))
	ginSvr.Handle(http.MethodGet, apitypes.APIDownloadLayer, h.HTTPWrapper(h.DownloadLayer))
	ginSvr.Handle(http.MethodGet, apitypes.APIRecorder, h.HTTPWrapper(h.Recorder))
	ginSvr.Handle(http.MethodGet, apitypes.APITorrentStatus, h.HTTPWrapper(h.TorrentStatus))

	ginSvr.Handle(http.MethodGet, apitypes.APITransferLayerTCP, h.HTTPWrapper(h.TransferLayerTCP))
}

func (h *CustomHandler) HTTPWrapper(f func(c *gin.Context) (interface{}, error)) func(c *gin.Context) {
	return func(c *gin.Context) {
		obj, err := f(c)
		if err != nil {
			c.String(http.StatusBadRequest, fmt.Sprintf("request '%s' failed: %s",
				c.Request.URL.Path, err.Error()))
			return
		}
		if obj == nil {
			// c.JSON(http.StatusOK, gin.H{})
			return
		}

		switch obj.(type) {
		case string:
			c.String(http.StatusOK, obj.(string))
		default:
			c.JSON(http.StatusOK, obj)
		}
	}
}
