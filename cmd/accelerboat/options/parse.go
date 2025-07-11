// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

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

}

const (
	defaultLogDir = "/data/accelerboat/logs"
)

func (op *AccelerBoatOption) checkLogConfig() error {
	if op.LogDir == "" {
		op.LogDir = defaultLogDir
	}
	if err := os.MkdirAll(op.LogDir, 0755); err != nil {
		return errors.Wrapf(err, "create log_dir '%s' failed", op.LogDir)
	}
	if op.LogMaxSize <= 0 {
		op.LogMaxSize = 100
	}
	if op.LogMaxBackups <= 0 {
		op.LogMaxBackups = 10
	}
	if op.LogMaxAge <= 0 {
		op.LogMaxAge = 30
	}
	logger.InitLogger(&logger.Option{
		Filename:   filepath.Join(op.LogDir, "accelerboat.log"),
		MaxSize:    op.LogMaxSize,
		MaxAge:     op.LogMaxAge,
		MaxBackups: op.LogMaxBackups,
	})
	return nil
}

func (op *AccelerBoatOption) checkFilePath() error {
	if err := os.MkdirAll(op.TransferPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", op.TransferPath)
	}
	if err := os.MkdirAll(op.StoragePath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", op.StoragePath)
	}
	if err := os.MkdirAll(op.SmallFilePath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", op.SmallFilePath)
	}
	// should remove torrentPath to avoid some cached files
	_ = os.RemoveAll(op.TorrentPath)
	if err := os.MkdirAll(op.TorrentPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", op.TorrentPath)
	}
	if err := os.MkdirAll(op.OCIPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", op.OCIPath)
	}
	return nil
}

const (
	// TwentyMB 20MB
	TwentyMB int64 = 20971520
	// TwoHundredMB 200MB
	TwoHundredMB int64 = 209715200
)

func (op *AccelerBoatOption) checkTorrentConfig() error {
	if op.TorrentThreshold < TwoHundredMB {
		op.TorrentThreshold = TwoHundredMB
	}
	if op.TorrentUploadLimit > 0 && op.TorrentUploadLimit < 1048576 {
		return errors.Errorf("upload limit '%d' too small, must >= 1048576(1MB/s)", op.TorrentUploadLimit)
	}
	if op.TorrentDownloadLimit > 0 && op.TorrentDownloadLimit < 1048576 {
		return errors.Errorf("download limit '%d' too small, must >= 1048576(1MB/s)", op.TorrentUploadLimit)
	}
	if op.DisableTorrent {

	}
}

func (op *AccelerBoatOption) checkCleanConfig() {

}
