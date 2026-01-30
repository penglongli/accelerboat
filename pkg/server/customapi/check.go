// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/juju/ratelimit"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/utils/formatutils"
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
	if !h.op.TorrentConfig.Enable || fileSize < h.op.TorrentConfig.Threshold*options.MB {
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
		// TODO: Currently, OCI layers are not being transferred using the BitTorrent protocol,
		// but rather via direct TCP transfer. We may consider using the BitTorrent protocol in the future.
		TorrentBase64: "",
		Located:       h.op.Address,
		LayerPath:     layerPath,
		FileSize:      fi.Size(),
	}, nil
}

var (
	// 默认 100MB 的 buffer
	defaultTransBuffer = int64(100 * 1048576)
)

const blockSize = 4096

func alignedBuffer(size int) []byte {
	// 分配额外空间以保证对齐
	b := make([]byte, size+blockSize)
	// 返回第一个对齐位置开始的 slice
	alignOffset := (blockSize - (uintptr(unsafe.Pointer(&b[0])) % blockSize)) % blockSize
	return b[alignOffset : alignOffset+uintptr(size)]
}

func (h *CustomHandler) TransferLayerTCP(c *gin.Context) (interface{}, error) {
	requestFile := c.Query("file")
	if requestFile == "" {
		return nil, errors.Errorf("quyer param 'file' cannot empty")
	}
	ctx := c.Request.Context()
	if fi, err := os.Stat(requestFile); err != nil {
		return nil, errors.Wrapf(err, "query file '%s' stat failed", requestFile)
	} else {
		logger.InfoContextf(ctx, "start read and write layer, file: %s, size: %s", requestFile,
			formatutils.FormatSize(fi.Size()))
	}

	file, err := os.OpenFile(requestFile, syscall.O_RDONLY|syscall.O_DIRECT, 0)
	if err != nil {
		logger.WarnContextf(ctx, "read file '%s' with directio failed: %s", requestFile, err.Error())
		http.ServeFile(c.Writer, c.Request, requestFile)
		return nil, nil
	}
	defer file.Close()

	bucket := ratelimit.NewBucketWithRate(float64(defaultTransBuffer), defaultTransBuffer)
	reader := bufio.NewReader(file)
	buf := alignedBuffer(32 * 1024) // 对齐的 32KB buffer
	rateReader := ratelimit.Reader(reader, bucket)
	if _, err = io.CopyBuffer(c.Writer, rateReader, buf); err != nil {
		return nil, errors.Wrapf(err, "io copy with file '%s' failed", requestFile)
	}
	logger.InfoContextf(ctx, "complete transfer layer, file: %s", requestFile)
	return nil, nil
}

func checkLocalLayer(filePath string) (int64, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return 0, errors.Wrapf(err, "stat layer file '%s' failed", filePath)
	}
	return fi.Size(), nil
}
