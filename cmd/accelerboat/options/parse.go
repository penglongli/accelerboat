// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"

	"github.com/penglongli/accelerboat/pkg/logger"
)

var (
	singleton = new(AccelerBoatOption)
)

// GlobalOptions returns the global option
func GlobalOptions() *AccelerBoatOption {
	return singleton
}

func Parse(configFile string) (*AccelerBoatOption, error) {
	bs, err := os.ReadFile(configFile)
	if err != nil {
		return nil, errors.Wrapf(err, "read config '%s' failed", configFile)
	}
	op := new(AccelerBoatOption)
	if err = json.Unmarshal(bs, op); err != nil {
		return nil, errors.Wrapf(err, "unmarshal config failed")
	}
	if err = op.checkLogConfig(); err != nil {
		return nil, errors.Wrapf(err, "check option log-config failed")
	}
	if err = op.checkStorageConfig(); err != nil {
		return nil, errors.Wrapf(err, "check option storage config failed")
	}
	if err = op.checkCleanConfig(); err != nil {
		return nil, errors.Wrapf(err, "check option clean config failed")
	}
	return op, nil
}

const (
	defaultLogDir = "/data/accelerboat/logs"
)

func (o *AccelerBoatOption) checkLogConfig() error {
	if o.LogConfig.LogDir == "" {
		o.LogConfig.LogDir = defaultLogDir
	}
	if err := os.MkdirAll(o.LogConfig.LogDir, 0755); err != nil {
		return errors.Wrapf(err, "create log_dir '%s' failed", o.LogConfig.LogDir)
	}
	if o.LogConfig.LogMaxSize <= 0 {
		o.LogConfig.LogMaxSize = 100
	}
	if o.LogConfig.LogMaxBackups <= 0 {
		o.LogConfig.LogMaxBackups = 10
	}
	if o.LogConfig.LogMaxAge <= 0 {
		o.LogConfig.LogMaxAge = 30
	}
	logger.InitLogger(&logger.Option{
		Filename:   filepath.Join(o.LogConfig.LogDir, "accelerboat.log"),
		MaxSize:    o.LogConfig.LogMaxSize,
		MaxAge:     o.LogConfig.LogMaxAge,
		MaxBackups: o.LogConfig.LogMaxBackups,
	})
	return nil
}

func (o *AccelerBoatOption) checkStorageConfig() error {
	if err := os.MkdirAll(o.StorageConfig.TransferPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.StorageConfig.TransferPath)
	}
	if err := os.MkdirAll(o.StorageConfig.DownloadPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.StorageConfig.DownloadPath)
	}
	if err := os.MkdirAll(o.StorageConfig.SmallFilePath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.StorageConfig.SmallFilePath)
	}
	if err := os.MkdirAll(o.StorageConfig.TorrentPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.StorageConfig.TorrentPath)
	}
	if err := os.MkdirAll(o.StorageConfig.OCIPath, 0600); err != nil {
		return errors.Wrapf(err, "create file-path '%s' failed", o.StorageConfig.OCIPath)
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
	if !o.TorrentConfig.Enable {
		return nil
	}
	if o.TorrentConfig.Threshold < TwoHundredMB {
		o.TorrentConfig.Threshold = TwoHundredMB
	}
	if o.TorrentConfig.UploadLimit > 0 && o.TorrentConfig.UploadLimit < 1048576 {
		o.TorrentConfig.UploadLimit = 1048576
	}
	if o.TorrentConfig.DownloadLimit > 0 && o.TorrentConfig.DownloadLimit < 1048576 {
		o.TorrentConfig.DownloadLimit = 1048576
	}
	return nil
}

func (o *AccelerBoatOption) checkCleanConfig() error {
	if o.CleanConfig.Cron == "" {
		logger.Infof("clean-config not set, no-need auto clean")
		return nil
	}
	parser := cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	schedule, err := parser.Parse(o.CleanConfig.Cron)
	if err != nil {
		return errors.Wrapf(err, "parse cron expression '%s' failed", o.CleanConfig.Cron)
	}
	if o.CleanConfig.Threshold < 10 {
		o.CleanConfig.Threshold = 10
	}
	if o.CleanConfig.RetainDays < 0 {
		o.CleanConfig.RetainDays = 0
	}

	logger.Infof("clean-config is set '%s', retain: %d day, size: %d GB print the next ten execution times:",
		o.CleanConfig.Cron, o.CleanConfig.RetainDays, o.CleanConfig.Threshold)
	currentTime := time.Now()
	for i := 0; i < 10; i++ {
		currentTime = schedule.Next(currentTime)
		logger.Infof("  [%d] %s", i, currentTime.Format("2006-01-02 15:04:05"))
	}
	return nil
}
