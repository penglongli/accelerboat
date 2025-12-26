// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package utils

import (
	"fmt"
	"os"
	"syscall"

	"github.com/pkg/errors"
)

// IsSparseFile check linux file is sparse file
func IsSparseFile(filePath string) (int64, int64, bool, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return 0, 0, false, errors.Wrap(err, "os.stat file '%s' failed")
	}

	// 获取系统底层文件信息（仅支持类Unix系统）
	sysStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false, fmt.Errorf("unsupported file system")
	}

	// 计算实际磁盘占用（字节）
	physicalSize := sysStat.Blocks * 512
	logicalSize := fileInfo.Size()

	// 判断是否为稀疏文件（逻辑大小 >> 物理占用）
	threshold := int64(10) // 阈值可调整
	return logicalSize, physicalSize, logicalSize > physicalSize*threshold, nil
}
