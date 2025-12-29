// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// CreateTarGz create tar.gz file
func CreateTarGz(srcDir, dstFile string) error {
	_ = os.RemoveAll(dstFile)
	dst, err := os.Create(dstFile)
	if err != nil {
		return err
	}
	defer dst.Close()
	gw := gzip.NewWriter(dst)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Walk through the source directory
	err = filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// NOTE: ignore tmp dir
		if fi.IsDir() && fi.Name() == "tmp" {
			return nil
		}

		var link string
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			if link, err = os.Readlink(file); err != nil {
				return err
			}
		}
		// Get the header for the current file
		header, err := tar.FileInfoHeader(fi, link)
		if err != nil {
			return err
		}
		// Set the correct path in the header
		relFilePath, err := filepath.Rel(srcDir, file)
		if err != nil {
			return err
		}
		header.Name = relFilePath

		// If it's not a directory, set the header size
		if !fi.Mode().IsDir() {
			header.Size = fi.Size()
		}
		// Write the header to the tar writer
		if err = tw.WriteHeader(header); err != nil {
			return err
		}
		// nothing more to do for non-regular
		if !fi.Mode().IsRegular() {
			return nil
		}
		// If it's a directory, we don't need to write its content
		if fi.Mode().IsDir() {
			return nil
		}

		// Open and copy the file's content
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})

	return err
}
