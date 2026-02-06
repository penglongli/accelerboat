// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package metrics

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const bytesPerGB = 1e9

// DirSizeBytes returns the total size in bytes of all files under dir (recursive).
func DirSizeBytes(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, errors.Wrapf(err, "walk dir %s", dir)
	}
	return total, nil
}

// UpdateDiskUsage sets DiskUsage gauge for each path. paths maps label (e.g. "transfer", "download")
// to directory path; the gauge value is the directory size in GB.
func UpdateDiskUsage(paths map[string]string) {
	for label, dirPath := range paths {
		if dirPath == "" {
			continue
		}
		size, err := DirSizeBytes(dirPath)
		if err != nil {
			DiskUsage.WithLabelValues(label).Set(0)
			continue
		}
		DiskUsage.WithLabelValues(label).Set(float64(size) / bytesPerGB)
	}
}
