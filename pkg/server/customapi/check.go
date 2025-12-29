// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
)

func (h *CustomHandler) CheckStaticLayer(c *gin.Context) (interface{}, error) {
	req := &apitypes.CheckStaticLayerRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	fileSize, err := checkLocalLayer(req.LayerPath)
	if err != nil {
		return nil, errors.Wrapf(err, "check local layer failed")
	}
	if fileSize != req.ExpectedContentLength {
		return nil, fmt.Errorf("local file '%s' content-length '%d', not same as expcted '%d'",
			req.LayerPath, fileSize, req.ExpectedContentLength)
	}
	resp := &apitypes.CheckStaticLayerResponse{
		Located:   h.op.Address,
		LayerPath: req.LayerPath,
		FileSize:  fileSize,
	}
	if !h.op.TorrentConfig.Enable || fileSize < h.op.TorrentConfig.Threshold {
		return resp, nil
	}

	ctx := c.Request.Context()
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	torrentBase64, err := h.torrentHandler.GenerateTorrent(timeoutCtx, req.Digest, req.LayerPath)
	if err != nil {
		logger.ErrorContextf(ctx, "generate torrent for '%s' failed: %s", req.LayerPath, err.Error())
	} else {
		resp.TorrentBase64 = torrentBase64
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
		return nil, errors.Wrapf(err, "generate oci layer failed")
	}
	var fi os.FileInfo
	if fi, err = os.Stat(layerPath); err != nil {
		return nil, errors.Wrapf(err, "stat oc-layer '%s' failed", layerPath)
	}
	return &apitypes.CheckOCILayerResponse{
		Located:   h.op.Address,
		LayerPath: layerPath,
		FileSize:  fi.Size(),
	}, nil
}

func (h *CustomHandler) TransferLayerTCP(c *gin.Context) (interface{}, error) {
	requestFile := c.Query("file")
	if requestFile == "" {
		return nil, errors.Errorf("quyer param 'file' cannot empty")
	}
	if _, err := os.Stat(requestFile); err != nil {
		return nil, errors.Wrapf(err, "query file '%s' stat failed", requestFile)
	}
	http.ServeFile(c.Writer, c.Request, requestFile)
	return nil, nil
}

func checkLocalLayer(filePath string) (int64, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return 0, errors.Wrapf(err, "stat layer file '%s' failed", filePath)
	}
	return fi.Size(), nil
}
