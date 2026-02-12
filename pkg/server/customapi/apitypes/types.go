// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package apitypes

import (
	"fmt"
	"time"
)

const (
	APIGetServiceToken  = "/customapi/service-token"
	APIHeadManifest     = "/customapi/head-manifest"
	APIGetManifest      = "/customapi/get-manifest"
	APICheckStaticLayer = "/customapi/check-static-layer"
	APICheckOCILayer    = "/customapi/check-oci-layer"
	APIGetLayerInfo     = "/customapi/get-layer-info"
	APIDownloadLayer    = "/customapi/download-layer"
	APITransferLayerTCP = "/customapi/transfer-layer-tcp"
	APIRecorder         = "/customapi/recorder"
	APITorrentStatus    = "/customapi/torrent-status"
	APIStats            = "/customapi/stats"
	APIMetrics          = "/customapi/metrics"
	APIConfig           = "/customapi/config"
	APIOCIImages        = "/customapi/oci-images"
)

var (
	NotPrintLog = map[string]struct{}{
		APIRecorder:      {},
		APITorrentStatus: {},
		APIStats:         {},
		APIMetrics:       {},
		APIConfig:        {},
		APIOCIImages:    {},
		"/metrics":       {},
	}
)

type GetServiceTokenRequest struct {
	OriginalHost    string              `json:"originalHost"`
	ServiceTokenUrl string              `json:"serviceTokenUrl"`
	Headers         map[string][]string `json:"headers"`
	Service         string              `json:"service"`
	Scope           string              `json:"scope"`
}

type RegistryAuthToken struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

type HeadManifestRequest struct {
	OriginalHost    string              `json:"originalHost"`
	HeadManifestUrl string              `json:"headManifestUrl"`
	Headers         map[string][]string `json:"headers"`
	Repo            string              `json:"repo"`
	Tag             string              `json:"tag"`
}

type HeadManifestResponse struct {
	Headers map[string][]string `json:"headers"`
}

// GetManifestRequest defines the request of GetManifest
type GetManifestRequest struct {
	OriginalHost string              `json:"originalHost"`
	ManifestUrl  string              `json:"manifestUrl"`
	Headers      map[string][]string `json:"headers"`
	Repo         string              `json:"repo"`
	Tag          string              `json:"tag"`
}

// DownloadLayerRequest defines the request of download layer
type DownloadLayerRequest struct {
	OriginalHost string              `json:"originalHost"`
	LayerUrl     string              `json:"layerUrl"`
	Headers      map[string][]string `json:"headers"`
	Repo         string              `json:"repo"`
	Digest       string              `json:"digest"`
}

// DownloadLayerResponse defines the response of download layer
type DownloadLayerResponse struct {
	TorrentBase64 string `json:"torrentBase64"`
	Located       string `json:"located"`
	FilePath      string `json:"filePath"`
	FileSize      int64  `json:"fileSize"`
}

func (resp *DownloadLayerResponse) ToJSONString() string {
	var torrent string
	if resp.TorrentBase64 != "" {
		torrent = "(too long not print)"
	} else {
		torrent = "(no-torrent)"
	}
	return fmt.Sprintf(`{"torrent": "%s", "located": "%s", "filePath": "%s", "fileSize": %d}`,
		torrent, resp.Located, resp.FilePath, resp.FileSize)
}

// CheckStaticLayerRequest defines the request of check static layer
type CheckStaticLayerRequest struct {
	OriginalHost          string `json:"originalHost"`
	Repo                  string `json:"repo"`
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

// CheckOCILayerRequest defines the request of CheckOCILayer
type CheckOCILayerRequest struct {
	Digest  string `json:"digest"`
	OCIType string `json:"ociType"`
}

// CheckOCILayerResponse defines the response of CheckOCILayer
type CheckOCILayerResponse struct {
	Located       string `json:"located"`
	LayerPath     string `json:"layerPath"`
	TorrentBase64 string `json:"torrentBase64"`
	FileSize      int64  `json:"fileSize"`
}
