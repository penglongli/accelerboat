// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package registry

import (
	"net/http"
	"time"

	"github.com/penglongli/accelerboat/pkg/metrics"
	"github.com/penglongli/accelerboat/pkg/recorder"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
)

func (p *upstreamProxy) recorderReverseProxy(req *http.Request) {
	if req.URL.Path == "/v2/" {
		return
	}
	recorder.Global.Record(recorder.Event{
		Type: recorder.EventTypeReverseProxy,
		Details: map[string]interface{}{
			"registry": p.originalHost, "method": req.Method, "path": req.URL.Path,
		},
	})
	metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeReverseProxy),
		"forwarded").Inc()
}

func (p *upstreamProxy) recorderServiceToken(start time.Time, master, service, scope string, err error) {
	duration := time.Since(start)
	metrics.RegistryRequestDurationSeconds.WithLabelValues(p.originalHost, string(recorder.EventTypeServiceToken)).
		Observe(duration.Seconds())
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeServiceToken,
			Details: map[string]interface{}{
				"registry": p.originalHost, "status": "error",
				"service": service, "scope": scope,
				"duration_ms": duration.Milliseconds(), "master": master, "error": err.Error(),
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeServiceToken),
			"error").Inc()
	} else {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeServiceToken,
			Details: map[string]interface{}{
				"registry": p.originalHost, "status": "success",
				"service": service, "scope": scope,
				"duration_ms": duration.Milliseconds(), "master": master,
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeServiceToken),
			"success").Inc()
	}
}

func (p *upstreamProxy) recorderHeadManifest(start time.Time, master, repo, tag string, err error) {
	duration := time.Since(start)
	metrics.RegistryRequestDurationSeconds.WithLabelValues(p.originalHost, string(recorder.EventTypeHeadManifest)).
		Observe(duration.Seconds())
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeHeadManifest,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "tag": tag,
				"status": "error", "duration_ms": duration.Milliseconds(), "error": err.Error(),
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeHeadManifest),
			"error").Inc()
	} else {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeHeadManifest,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "tag": tag,
				"status": "success", "duration_ms": duration.Milliseconds(), "master": master,
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeHeadManifest),
			"success").Inc()
	}
}

func (p *upstreamProxy) recorderGetManifest(start time.Time, master, repo, tag, manifest string, err error) {
	duration := time.Since(start)
	metrics.RegistryRequestDurationSeconds.WithLabelValues(p.originalHost, string(recorder.EventTypeGetManifest)).
		Observe(duration.Seconds())
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeGetManifest,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "tag": tag,
				"status": "error", "duration_ms": duration.Milliseconds(), "error": err.Error(),
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeGetManifest),
			"error").Inc()
	} else {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeGetManifest,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "tag": tag,
				"status": "success", "duration_ms": duration.Milliseconds(), "master": master,
				"data": manifest,
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeGetManifest),
			"success").Inc()
	}
}

func (p *upstreamProxy) recorderServeBlobFromLocal(start time.Time, repo, digest string, size int64, err error) {
	duration := time.Since(start)
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventServeBlobFromLocal,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "digest": digest,
				"status": "error", "duration_ms": duration.Milliseconds(),
				"error": "serve local failed",
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventServeBlobFromLocal),
			"error").Inc()
	} else {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventServeBlobFromLocal,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "digest": digest,
				"status": "success", "duration_ms": duration.Milliseconds(), "size": size,
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventServeBlobFromLocal),
			"success").Inc()
		metrics.TransferSize.WithLabelValues("serve_blob_from_local").Add(float64(size) / 1e9)
	}
}

func (p *upstreamProxy) recorderGetBlobFromMaster(start time.Time, master, repo, digest string,
	layerResp *apitypes.DownloadLayerResponse, err error) {
	duration := time.Since(start)
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeGetBlobFromMaster,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "digest": digest,
				"status": "error", "duration_ms": duration.Milliseconds(), "error": err.Error(),
				"master": master,
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost,
			string(recorder.EventTypeGetBlobFromMaster), "error").Inc()
	} else {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeGetBlobFromMaster,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "digest": digest,
				"status": "success", "duration_ms": duration.Milliseconds(),
				"master": master, "data": layerResp.ToJSONString(),
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost,
			string(recorder.EventTypeGetBlobFromMaster), "success").Inc()
	}
}

func (p *upstreamProxy) recorderDownloadBlobByTCP(start time.Time, repo, digest string,
	layerResp *apitypes.DownloadLayerResponse, err error) {
	duration := time.Since(start)
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeDownloadBlobByTCP,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "digest": digest,
				"status": "error", "duration_ms": duration.Milliseconds(), "error": err.Error(),
				"data": layerResp.ToJSONString(),
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeDownloadBlobByTCP),
			"error").Inc()
	} else {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeDownloadBlobByTCP,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "digest": digest,
				"status": "success", "duration_ms": duration.Milliseconds(),
				"data": layerResp.ToJSONString(),
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeDownloadBlobByTCP),
			"success").Inc()
		metrics.TransferSize.WithLabelValues("download_by_tcp").Add(float64(layerResp.FileSize) / 1e9)
	}
}

func (p *upstreamProxy) recorderDownloadBlobByTorrent(start time.Time, repo, digest string,
	layerResp *apitypes.DownloadLayerResponse, err error) {
	duration := time.Since(start)
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeDownloadBlobByTorrent,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "digest": digest,
				"status": "error", "duration_ms": duration.Milliseconds(), "error": err.Error(),
				"data": layerResp.ToJSONString(),
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeDownloadBlobByTorrent),
			"error").Inc()
	} else {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeDownloadBlobByTorrent,
			Details: map[string]interface{}{
				"registry": p.originalHost, "repo": repo, "digest": digest,
				"status": "success", "duration_ms": duration.Milliseconds(),
				"data": layerResp.ToJSONString(),
			},
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeDownloadBlobByTorrent),
			"success").Inc()
		metrics.TransferSize.WithLabelValues("download_by_torrent").Add(float64(layerResp.FileSize) / 1e9)
	}
}
