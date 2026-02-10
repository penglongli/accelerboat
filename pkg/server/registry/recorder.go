// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package registry

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/penglongli/accelerboat/pkg/metrics"
	"github.com/penglongli/accelerboat/pkg/recorder"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
)

func (p *upstreamProxy) recorderReverseProxy(ctx context.Context, req *http.Request) {
	if req.URL.Path == "/v2/" {
		return
	}
	recorder.Global.Record(ctx, recorder.Event{
		Type:        recorder.EventTypeReverseProxy,
		EventStatus: recorder.Normal,
		Details: map[string]interface{}{
			"registry": p.originalHost, "method": req.Method, "path": req.URL.Path,
		},
		Message: fmt.Sprintf("Reverse proxy request"),
	})
	metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeReverseProxy),
		"forwarded").Inc()
}

func (p *upstreamProxy) recorderReverseProxyFailed(ctx context.Context, req *http.Request, err error) {
	if req.URL.Path == "/v2/" {
		return
	}
	recorder.Global.Record(ctx, recorder.Event{
		Type:        recorder.EventTypeReverseProxy,
		EventStatus: recorder.Warning,
		Details: map[string]interface{}{
			"registry": p.originalHost, "method": req.Method, "path": req.URL.Path,
		},
		Message: fmt.Sprintf("Reverse proxy request occurred error: %s", err.Error()),
	})
	metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeReverseProxy),
		"error").Inc()
}

func (p *upstreamProxy) recorderServiceToken(ctx context.Context, start time.Time, master, service, scope string,
	err error) {
	duration := time.Since(start)
	metrics.RegistryRequestDurationSeconds.WithLabelValues(p.originalHost, string(recorder.EventTypeServiceToken)).
		Observe(duration.Seconds())
	details := map[string]interface{}{
		"registry": p.originalHost, "service": service, "scope": scope,
		"duration_ms": duration.Milliseconds(), "master": master,
	}
	if err != nil {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeServiceToken,
			EventStatus: recorder.Warning,
			Details:     details,
			Message:     fmt.Sprintf("Get servicetoken from master failed: %s", err.Error()),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeServiceToken),
			"error").Inc()
	} else {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeServiceToken,
			EventStatus: recorder.Normal,
			Details:     details,
			Message:     fmt.Sprintf("Get servicetoken from master success"),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeServiceToken),
			"success").Inc()
	}
}

func (p *upstreamProxy) recorderHeadManifest(ctx context.Context, start time.Time, master,
	repo, tag string, err error) {
	duration := time.Since(start)
	metrics.RegistryRequestDurationSeconds.WithLabelValues(p.originalHost, string(recorder.EventTypeHeadManifest)).
		Observe(duration.Seconds())
	details := map[string]interface{}{
		"registry": p.originalHost, "repo": repo, "tag": tag, "duration_ms": duration.Milliseconds(),
		"master": master,
	}
	if err != nil {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeHeadManifest,
			EventStatus: recorder.Warning,
			Details:     details,
			Message:     fmt.Sprintf("Head manifest from master failed: %s", err.Error()),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeHeadManifest),
			"error").Inc()
	} else {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeHeadManifest,
			EventStatus: recorder.Normal,
			Details:     details,
			Message:     fmt.Sprintf("Head manifest from master success"),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeHeadManifest),
			"success").Inc()
	}
}

func (p *upstreamProxy) recorderGetManifest(ctx context.Context, start time.Time, master, repo, tag,
	manifest string, err error) {
	duration := time.Since(start)
	metrics.RegistryRequestDurationSeconds.WithLabelValues(p.originalHost, string(recorder.EventTypeGetManifest)).
		Observe(duration.Seconds())
	details := map[string]interface{}{
		"registry": p.originalHost, "repo": repo, "tag": tag, "duration_ms": duration.Milliseconds(),
		"master": master,
	}
	if err != nil {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeGetManifest,
			EventStatus: recorder.Warning,
			Details:     details,
			Message:     fmt.Sprintf("Get manifest from master failed: %s", err.Error()),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeGetManifest),
			"error").Inc()
	} else {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeGetManifest,
			EventStatus: recorder.Normal,
			Details:     details,
			Message:     fmt.Sprintf("Get manifest from master success"),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeGetManifest),
			"success").Inc()
	}
}

func (p *upstreamProxy) recorderServeBlobFromLocal(ctx context.Context, start time.Time, repo, digest string,
	size int64, err error) {
	duration := time.Since(start)
	details := map[string]interface{}{
		"registry": p.originalHost, "repo": repo, "digest": digest, "duration_ms": duration.Milliseconds(),
		"size": size,
	}
	if err != nil {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventServeBlobFromLocal,
			EventStatus: recorder.Warning,
			Details:     details,
			Message:     fmt.Sprintf("Serve blob to client from local failed: %s", err.Error()),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventServeBlobFromLocal),
			"error").Inc()
	} else {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventServeBlobFromLocal,
			EventStatus: recorder.Normal,
			Details:     details,
			Message:     fmt.Sprintf("Serve blob to client from local success"),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventServeBlobFromLocal),
			"success").Inc()
		metrics.TransferSize.WithLabelValues("serve_blob_from_local").Add(float64(size) / 1e9)
	}
}

func (p *upstreamProxy) recorderGetBlobFromMaster(ctx context.Context, start time.Time, master, repo, digest string,
	layerResp *apitypes.DownloadLayerResponse, err error) {
	duration := time.Since(start)
	details := map[string]interface{}{
		"registry": p.originalHost, "repo": repo, "digest": digest,
		"duration_ms": duration.Milliseconds(), "master": master,
	}
	if err != nil {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeGetBlobFromMaster,
			EventStatus: recorder.Warning,
			Details:     details,
			Message:     fmt.Sprintf("Get blob from master failed: %s", err.Error()),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost,
			string(recorder.EventTypeGetBlobFromMaster), "error").Inc()
	} else {
		details["data"] = layerResp.ToJSONString()
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeGetBlobFromMaster,
			EventStatus: recorder.Normal,
			Details:     details,
			Message:     fmt.Sprintf("Get blob from master success"),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost,
			string(recorder.EventTypeGetBlobFromMaster), "success").Inc()
	}
}

func (p *upstreamProxy) recorderDownloadBlobByTCP(ctx context.Context, start time.Time, repo, digest string,
	layerResp *apitypes.DownloadLayerResponse, err error) {
	duration := time.Since(start)
	details := map[string]interface{}{
		"registry": p.originalHost, "repo": repo, "digest": digest,
		"duration_ms": duration.Milliseconds(),
		"data":        layerResp.ToJSONString(),
	}
	if err != nil {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeDownloadBlobByTCP,
			EventStatus: recorder.Warning,
			Details:     details,
			Message:     fmt.Sprintf("Download blob by tcp failed: %s", err.Error()),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeDownloadBlobByTCP),
			"error").Inc()
	} else {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeDownloadBlobByTCP,
			EventStatus: recorder.Normal,
			Details:     details,
			Message:     fmt.Sprintf("Download blob by tcp success"),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeDownloadBlobByTCP),
			"success").Inc()
		metrics.TransferSize.WithLabelValues("download_by_tcp").Add(float64(layerResp.FileSize) / 1e9)
	}
}

func (p *upstreamProxy) recorderDownloadBlobByTorrent(ctx context.Context, start time.Time, repo, digest string,
	layerResp *apitypes.DownloadLayerResponse, err error) {
	duration := time.Since(start)
	details := map[string]interface{}{
		"registry": p.originalHost, "repo": repo, "digest": digest,
		"duration_ms": duration.Milliseconds(),
		"data":        layerResp.ToJSONString(),
	}
	if err != nil {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeDownloadBlobByTorrent,
			EventStatus: recorder.Warning,
			Details:     details,
			Message:     fmt.Sprintf("Download blob by torrent failed: %s", err.Error()),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeDownloadBlobByTorrent),
			"error").Inc()
	} else {
		recorder.Global.Record(ctx, recorder.Event{
			Type:        recorder.EventTypeDownloadBlobByTorrent,
			EventStatus: recorder.Normal,
			Details:     details,
			Message:     fmt.Sprintf("Download blob by torrent success"),
		})
		metrics.RegistryRequestsTotal.WithLabelValues(p.originalHost, string(recorder.EventTypeDownloadBlobByTorrent),
			"success").Inc()
		metrics.TransferSize.WithLabelValues("download_by_torrent").Add(float64(layerResp.FileSize) / 1e9)
	}
}
