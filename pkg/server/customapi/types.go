// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/store"
)

const (
	APIHeadDigest       = "/customapi/head-digest"
	APIGetManifest      = "/customapi/get-manifest"
	APICheckStaticLayer = "/customapi/check-static-layer"
	APICheckOCILayer    = "/customapi/check-oci-layer"
	APIGetLayerInfo     = "/customapi/get-layer-info"
	APIDownloadLayer    = "/customapi/download-layer"
	APITransferLayerTCP = "/customapi/transfer-layer-tcp"
	APIRecorder         = "/customapi/recorder"
	APITorrentStatus    = "/customapi/torrent-status"
)

// CustomHandler 定义了一系列的方法，用于对外提供服务. 通常是由普通 node 来调用 master 的这些对外接口能力.
type CustomHandler struct {
	op         *options.AccelerBoatOption
	cacheStore store.CacheStore
}

func NewCustomHandler(op *options.AccelerBoatOption) *CustomHandler {
	return &CustomHandler{
		op: op,
	}
}

func (h *CustomHandler) Register(ginSvr *gin.Engine) {
	ginSvr.Handle(http.MethodPost, APIHeadDigest, h.RegistryHeadDigest)
	ginSvr.Handle(http.MethodPost, APIGetManifest, h.RegistryGetManifest)
	ginSvr.Handle(http.MethodGet, APICheckStaticLayer, h.CheckOCILayer)
	ginSvr.Handle(http.MethodGet, APICheckOCILayer, h.CheckOCILayer)
	ginSvr.Handle(http.MethodPost, APIGetLayerInfo, h.GetLayerInfo)
	ginSvr.Handle(http.MethodGet, APIDownloadLayer, h.DownloadLayer)
	ginSvr.Handle(http.MethodGet, APITransferLayerTCP, h.TransferLayerTCP)
	ginSvr.Handle(http.MethodGet, APIRecorder, h.Recorder)
	ginSvr.Handle(http.MethodGet, APITorrentStatus, h.TorrentStatus)
}

type HeadDigestRequest struct {
	OriginalHost  string              `json:"originalHost"`
	HeadDigestUrl string              `json:"headDigestUrl"`
	Headers       map[string][]string `json:"headers"`
}

type HeadDigestResponse struct {
	Headers map[string][]string `json:"headers"`
}

func (h *CustomHandler) RegistryHeadDigest(c *gin.Context) {

}

// GetManifestRequest defines the request of GetManifest
type GetManifestRequest struct {
	OriginalHost string `json:"originalHost"`
	ManifestUrl  string `json:"manifestUrl"`
	Repo         string `json:"repo"`
	Tag          string `json:"tag"`
	BearerToken  string `json:"bearerToken"`
}

func (h *CustomHandler) RegistryGetManifest(c *gin.Context) {

}

// CheckStaticLayerRequest defines the request of check static layer
type CheckStaticLayerRequest struct {
	Digest                string `json:"digest"`
	LayerPath             string `json:"path"`
	ExpectedContentLength int64  `json:"expectedContentLength"`
}

// CheckStaticLayerResponse defines the response of CheckStaticLayer
type CheckStaticLayerResponse struct {
	Located       string `json:"located"`
	LayerPath     string `json:"layerPath"`
	TorrentBase64 string `json:"torrentBase64"`
	FileSize      int64  `json:"fileSize"`
}

func (h *CustomHandler) CheckStaticLayer(c *gin.Context) {

}

// CheckOCILayerRequest defines the request of CheckOCILayer
type CheckOCILayerRequest struct {
	Digest  string `json:"digest"`
	OCIType string `json:"ociType"`
}

// CheckOCILayerResponse defines the response of CheckOCILayer
type CheckOCILayerResponse struct {
	Located   string `json:"located"`
	LayerPath string `json:"layerPath"`
	FileSize  int64  `json:"fileSize"`
}

func (h *CustomHandler) CheckOCILayer(c *gin.Context) {

}

// DownloadLayerRequest defines the request of download layer
type DownloadLayerRequest struct {
	OriginalHost string `json:"originalHost"`
	LayerUrl     string `json:"layerUrl"`
	BearerToken  string `json:"bearerToken"`
}

// DownloadLayerResponse defines the response of download layer
type DownloadLayerResponse struct {
	TorrentBase64 string `json:"torrentBase64"`
	Located       string `json:"located"`
	FilePath      string `json:"filePath"`
	FileSize      int64  `json:"fileSize"`
}

func (h *CustomHandler) GetLayerInfo(c *gin.Context) {

}

func (h *CustomHandler) DownloadLayer(c *gin.Context) {

}

func (h *CustomHandler) TransferLayerTCP(c *gin.Context) {

}
func (h *CustomHandler) Recorder(c *gin.Context) {

}

func (h *CustomHandler) TorrentStatus(c *gin.Context) {}
