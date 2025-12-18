// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/pkg/logger"
)

func Parse(configFile string) (*AccelerBoatOption, error) {
	bs, err := os.ReadFile(configFile)
	if err != nil {
		return nil, errors.Wrapf(err, "read config '%s' failed", configFile)
	}
	op := new(AccelerBoatOption)
	if err = json.Unmarshal(bs, op); err != nil {
		return nil, errors.Wrapf(err, "unmarshal config failed")
	}
	return op, nil
}

const (
	defaultLogDir = "/data/accelerboat/logs"
)

func (o *AccelerBoatOption) checkLogConfig() error {
	if o.LogDir == "" {
		o.LogDir = defaultLogDir
	}
	if err := os.MkdirAll(o.LogDir, 0755); err != nil {
		return errors.Wrapf(err, "create log_dir '%s' failed", o.LogDir)
	}
	if o.LogMaxSize <= 0 {
		o.LogMaxSize = 100
	}
	if o.LogMaxBackups <= 0 {
		o.LogMaxBackups = 10
	}
	if o.LogMaxAge <= 0 {
		o.LogMaxAge = 30
	}
	logger.InitLogger(&logger.Option{
		Filename:   filepath.Join(o.LogDir, "accelerboat.log"),
		MaxSize:    o.LogMaxSize,
		MaxAge:     o.LogMaxAge,
		MaxBackups: o.LogMaxBackups,
	})
	return nil
}

func (o *AccelerBoatOption) checkFilePath() error {
	if err := os.MkdirAll(o.TransferPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.TransferPath)
	}
	if err := os.MkdirAll(o.StoragePath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.StoragePath)
	}
	if err := os.MkdirAll(o.SmallFilePath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.SmallFilePath)
	}
	// should remove torrentPath to avoid some cached files
	_ = os.RemoveAll(o.TorrentPath)
	if err := os.MkdirAll(o.TorrentPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.TorrentPath)
	}
	if err := os.MkdirAll(o.OCIPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.OCIPath)
	}
	return nil
}

const (
	// TwentyMB 20MB
	TwentyMB int64 = 20971520
	// TwoHundredMB 200MB
	TwoHundredMB int64 = 209715200
)

func (o *AccelerBoatOption) checkTorrentConfig() error {
	if o.TorrentThreshold < TwoHundredMB {
		o.TorrentThreshold = TwoHundredMB
	}
	if o.TorrentUploadLimit > 0 && o.TorrentUploadLimit < 1048576 {
		return errors.Errorf("upload limit '%d' too small, must >= 1048576(1MB/s)", o.TorrentUploadLimit)
	}
	if o.TorrentDownloadLimit > 0 && o.TorrentDownloadLimit < 1048576 {
		return errors.Errorf("download limit '%d' too small, must >= 1048576(1MB/s)", o.TorrentUploadLimit)
	}
	if o.DisableTorrent {
		return nil
	}
	return nil
}

func (o *AccelerBoatOption) checkCleanConfig() {

}
