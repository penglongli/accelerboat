// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/metrics"
	"github.com/penglongli/accelerboat/pkg/recorder"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/utils/httpfile"
)

func (h *CustomHandler) CheckStaticLayer(c *gin.Context) (interface{}, error) {
	req := &apitypes.CheckStaticLayerRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	fileSize, err := checkLocalLayer(req.LayerPath)
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeCheckStatic, Details: map[string]interface{}{
				"registry": req.OriginalHost, "repo": req.Repo, "digest": req.Digest,
				"status": "error", "error": err.Error(),
			},
		})
		return nil, errors.Wrapf(err, "check local layer failed")
	}
	if fileSize != req.ExpectedContentLength {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeCheckStatic, Details: map[string]interface{}{
				"registry": req.OriginalHost, "repo": req.Repo, "digest": req.Digest,
				"status": "error", "error": "content-length mismatch",
			},
		})
		return nil, fmt.Errorf("local file '%s' content-length '%d', not same as expcted '%d'",
			req.LayerPath, fileSize, req.ExpectedContentLength)
	}
	resp := &apitypes.CheckStaticLayerResponse{
		Located:   h.op.Address,
		LayerPath: req.LayerPath,
		FileSize:  fileSize,
	}
	if !h.op.TorrentConfig.Enable || fileSize < h.op.TorrentConfig.Threshold*options.MB {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeCheckStatic, Details: map[string]interface{}{
				"registry": req.OriginalHost, "repo": req.Repo, "digest": req.Digest,
				"status": "success", "located": h.op.Address, "file_size": fileSize,
			},
		})
		return resp, nil
	}

	ctx := c.Request.Context()
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	torrentBase64, err := h.torrentHandler.GenerateTorrent(timeoutCtx, req.Digest, req.LayerPath)
	if err != nil {
		logger.ErrorContextf(ctx, "generate torrent for '%s' failed: %s", req.LayerPath, err.Error())
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeCheckStatic, Details: map[string]interface{}{
				"registry": req.OriginalHost, "repo": req.Repo, "digest": req.Digest,
				"status": "success", "located": h.op.Address, "file_size": fileSize, "torrent": "failed",
			},
		})
	} else {
		resp.TorrentBase64 = torrentBase64
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeCheckStatic, Details: map[string]interface{}{
				"registry": req.OriginalHost, "repo": req.Repo, "digest": req.Digest,
				"status": "success", "located": h.op.Address, "file_size": fileSize,
			},
		})
	}
	return resp, nil
}

func (h *CustomHandler) CheckOCILayer(c *gin.Context) (interface{}, error) {
	req := &apitypes.CheckOCILayerRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	ctx := c.Request.Context()
	layerPath, err := h.ociScanner.GenerateLayer(ctx, req.OCIType, req.Digest)
	if err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeCheckOCI, Details: map[string]interface{}{
				"digest": req.Digest, "oci_type": req.OCIType, "status": "error", "error": err.Error(),
			},
		})
		return nil, errors.Wrapf(err, "generate oci layer failed")
	}
	var fi os.FileInfo
	if fi, err = os.Stat(layerPath); err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeCheckOCI, Details: map[string]interface{}{
				"digest": req.Digest, "oci_type": req.OCIType, "status": "error", "error": err.Error(),
			},
		})
		return nil, errors.Wrapf(err, "stat oc-layer '%s' failed", layerPath)
	}
	recorder.Global.Record(recorder.Event{
		Type: recorder.EventTypeCheckOCI, Details: map[string]interface{}{
			"digest": req.Digest, "oci_type": req.OCIType, "status": "success", "located": h.op.Address, "file_size": fi.Size(),
		},
	})
	return &apitypes.CheckOCILayerResponse{
		// TODO: Currently, OCI layers are not being transferred using the BitTorrent protocol,
		// but rather via direct TCP transfer. We may consider using the BitTorrent protocol in the future.
		TorrentBase64: "",
		Located:       h.op.Address,
		LayerPath:     layerPath,
		FileSize:      fi.Size(),
	}, nil
}

func (h *CustomHandler) TransferLayerTCP(c *gin.Context) (interface{}, error) {
	requestFile := c.Query("file")
	if requestFile == "" {
		return nil, errors.Errorf("quyer param 'file' cannot empty")
	}
	ctx := c.Request.Context()
	var fileSize int64
	if fi, err := os.Stat(requestFile); err == nil && !fi.IsDir() {
		fileSize = fi.Size()
	}
	if err := httpfile.HTTPServeFile(ctx, c.Writer, c.Request, requestFile); err != nil {
		recorder.Global.Record(recorder.Event{
			Type: recorder.EventTypeTransferLayer, Details: map[string]interface{}{
				"file": requestFile, "status": "error", "error": err.Error(),
			},
		})
		return nil, err
	}
	recorder.Global.Record(recorder.Event{
		Type: recorder.EventTypeTransferLayer, Details: map[string]interface{}{
			"file": requestFile, "status": "success",
		},
	})
	if fileSize > 0 {
		metrics.TransferSize.WithLabelValues("serve_blob_by_tcp").Add(float64(fileSize) / 1e9)
	}
	return nil, nil
}

func checkLocalLayer(filePath string) (int64, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return 0, errors.Wrapf(err, "stat layer file '%s' failed", filePath)
	}
	return fi.Size(), nil
}
