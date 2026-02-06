// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package httpfile

import (
	"context"
	"io"
	"net/http"
	"os"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/utils/formatutils"
)

const blockSize = 4096

func alignedBuffer(size int) []byte {
	// 分配额外空间以保证对齐
	b := make([]byte, size+blockSize)
	// 返回第一个对齐位置开始的 slice
	alignOffset := (blockSize - (uintptr(unsafe.Pointer(&b[0])) % blockSize)) % blockSize
	return b[alignOffset : alignOffset+uintptr(size)]
}

func HTTPServeFile(ctx context.Context, rw http.ResponseWriter, req *http.Request, reqFile string) error {
	if fi, err := os.Stat(reqFile); err != nil {
		return errors.Wrapf(err, "query file '%s' stat failed", reqFile)
	} else {
		logger.InfoContextf(ctx, "start read and write layer, file: %s, size: %s", reqFile,
			formatutils.FormatSize(fi.Size()))
	}

	file, err := os.OpenFile(reqFile, syscall.O_RDONLY|syscall.O_DIRECT, 0)
	if err != nil {
		logger.WarnContextf(ctx, "read file '%s' with directio failed: %s", reqFile, err.Error())
		http.ServeFile(rw, req, reqFile)
		return nil
	}
	defer file.Close()

	buf := alignedBuffer(32 * 1024) // 对齐的 32KB buffer
	if _, err = io.CopyBuffer(rw, file, buf); err != nil {
		return errors.Wrapf(err, "io copy with file '%s' failed", reqFile)
	}
	logger.InfoContextf(ctx, "complete transfer layer, file: %s", reqFile)
	return nil
}
