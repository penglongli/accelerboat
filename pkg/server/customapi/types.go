// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/bittorrent"
	"github.com/penglongli/accelerboat/pkg/ociscan"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/store"
	"github.com/penglongli/accelerboat/pkg/utils/lock"
)

// CustomHandler defines a set of methods for external services. It is typically used by regular nodes to call
// the master's external API capabilities.
type CustomHandler struct {
	op         *options.AccelerBoatOption
	cacheStore store.CacheStore

	authLock               lock.Interface
	authTokens             *cache.Cache
	headManifestLock       lock.Interface
	headManifests          *cache.Cache
	getManifestLock        lock.Interface
	manifests              *cache.Cache
	layerContentLengthLock lock.Interface
	layerContentLengths    *cache.Cache
	downloadLayerLock      lock.Interface

	staticLayerRefer map[string]map[string]int64
	ociLayerRefer    map[string]map[string]int64

	nodeDownloadLock  sync.Mutex
	nodeDownloadTasks map[string]int

	torrentHandler *bittorrent.TorrentHandler
	ociScanner     *ociscan.ScanHandler
}

// NewCustomHandler creates a CustomHandler with the given options, torrent handler, and OCI scanner.
func NewCustomHandler(op *options.AccelerBoatOption, torrentHandler *bittorrent.TorrentHandler,
	ociScanner *ociscan.ScanHandler) *CustomHandler {
	return &CustomHandler{
		op:                     op,
		cacheStore:             store.GlobalRedisStore(),
		authLock:               lock.NewLocalLock(),
		authTokens:             cache.New(0, 5*time.Second),
		headManifestLock:       lock.NewLocalLock(),
		headManifests:          cache.New(0, 5*time.Second),
		getManifestLock:        lock.NewLocalLock(),
		manifests:              cache.New(0, 5*time.Second),
		layerContentLengthLock: lock.NewLocalLock(),
		layerContentLengths:    cache.New(0, 5*time.Second),
		downloadLayerLock:      lock.NewLocalLock(),
		nodeDownloadTasks:      make(map[string]int),
		staticLayerRefer:       make(map[string]map[string]int64),
		ociLayerRefer:          make(map[string]map[string]int64),
		torrentHandler:         torrentHandler,
		ociScanner:             ociScanner,
	}
}

// Register mounts all custom API routes on the given Gin engine.
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

	ginSvr.Handle(http.MethodGet, apitypes.APIStats, h.HTTPWrapperWithOutput(h.Stats))
	ginSvr.Handle(http.MethodGet, apitypes.APIMetrics, h.HTTPWrapperWithOutput(h.Metrics))
	ginSvr.Handle(http.MethodGet, apitypes.APIConfig, h.HTTPWrapperWithOutput(h.Config))
}

// HTTPWrapperWithOutput wraps handlers for stats/metrics/config etc.: if query param output=json
//
//	is set, responds with JSON; otherwise returns formatted text.
func (h *CustomHandler) HTTPWrapperWithOutput(f func(c *gin.Context) (interface{}, string, error)) func(c *gin.Context) {
	return func(c *gin.Context) {
		jsonData, text, err := f(c)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		if c.Query("output") == "json" {
			c.JSON(http.StatusOK, jsonData)
			return
		}
		c.String(http.StatusOK, text)
	}
}

// HTTPWrapper wraps a handler that returns (interface{}, error) and responds with JSON or string accordingly.
func (h *CustomHandler) HTTPWrapper(f func(c *gin.Context) (interface{}, error)) func(c *gin.Context) {
	return func(c *gin.Context) {
		obj, err := f(c)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
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
