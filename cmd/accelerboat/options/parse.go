// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Tencent/bk-bcs/bcs-common/common/blog"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/utils"
)

var (
	prev      = new(AccelerBoatOption)
	singleton = new(AccelerBoatOption)
)

// GlobalOptions returns the global option
func GlobalOptions() *AccelerBoatOption {
	return singleton
}

func changeOption(op *AccelerBoatOption) {
	// initialized for the first time
	if singleton == nil || prev == nil {
		_ = utils.DeepCopyStruct(op, singleton)
		_ = utils.DeepCopyStruct(op, prev)
		logger.InitLogger(&logger.Option{
			Filename:   filepath.Join(op.LogConfig.LogDir, "accelerboat.log"),
			MaxSize:    op.LogConfig.LogMaxSize,
			MaxAge:     op.LogConfig.LogMaxAge,
			MaxBackups: op.LogConfig.LogMaxBackups,
		})
		return
	}

	_ = utils.DeepCopyStruct(singleton, prev)
	_ = utils.DeepCopyStruct(op, singleton)
	if prev.LogConfig.LogDir != singleton.LogConfig.LogDir ||
		prev.LogConfig.LogMaxSize != singleton.LogConfig.LogMaxSize ||
		prev.LogConfig.LogMaxAge != singleton.LogConfig.LogMaxAge ||
		prev.LogConfig.LogMaxBackups != singleton.LogConfig.LogMaxBackups {
		logger.InitLogger(&logger.Option{
			Filename:   filepath.Join(op.LogConfig.LogDir, "accelerboat.log"),
			MaxSize:    op.LogConfig.LogMaxSize,
			MaxAge:     op.LogConfig.LogMaxAge,
			MaxBackups: op.LogConfig.LogMaxBackups,
		})
	}

	// if the singleton is empty, it means it's being initialized for the first time
	if singleton == nil {
		_ = utils.DeepCopyStruct(op, singleton)
	} else {
		_ = utils.DeepCopyStruct(singleton, prev)
		_ = utils.DeepCopyStruct(op, singleton)
	}
	if prev == nil {
		// initialized for the first time
		_ = utils.DeepCopyStruct(op, prev)
		logger.InitLogger(&logger.Option{
			Filename:   filepath.Join(op.LogConfig.LogDir, "accelerboat.log"),
			MaxSize:    op.LogConfig.LogMaxSize,
			MaxAge:     op.LogConfig.LogMaxAge,
			MaxBackups: op.LogConfig.LogMaxBackups,
		})
	} else {

	}
	logger.Infof("parsed options: %s", string(utils.ToJson(op)))
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
	if err = op.checkServiceDiscovery(); err != nil {
		return nil, errors.Wrapf(err, "check option service discovery failed")
	}
	if err = op.checkPreferConfig(); err != nil {
		return nil, errors.Wrapf(err, "check option prefer config failed")
	}
	if err = op.checkTorrentConfig(); err != nil {
		return nil, errors.Wrapf(err, "check option torrent config failed")
	}
	if err = op.checkExternalConfig(); err != nil {
		return nil, errors.Wrapf(err, "check option external config failed")
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

func (o *AccelerBoatOption) checkServiceDiscovery() error {
	if o.ServiceDiscovery.ServiceNamespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}
	if o.ServiceDiscovery.ServiceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	config, err := rest.InClusterConfig()
	o.k8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Wrapf(err, "create kubernetes client failed")
	}
	return nil
}

var (
	labelSelectorRegexp, _ = regexp.Compile(`^[^=,]+=[^=,]+(,[^=,]+=[^=,]+)*$`)
)

func (o *AccelerBoatOption) checkPreferConfig() error {
	if o.PreferConfig == nil {
		return nil
	}
	if o.PreferConfig.PreferNodes == nil {
		return nil
	}
	selector := o.PreferConfig.PreferNodes.LabelSelectors
	if selector == "" {
		return nil
	}
	if !labelSelectorRegexp.MatchString(selector) {
		return fmt.Errorf("invalid preferNodes.labelSelector '%s'", selector)
	}
	return nil
}

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

func (o *AccelerBoatOption) checkExternalConfig() error {
	if o.ExternalConfig.HTTPProxy != "" {
		var err error
		if o.ExternalConfig.HTTPProxyUrl, err = url.Parse(o.ExternalConfig.HTTPProxy); err != nil {
			return errors.Wrapf(err, "http_proxy '%s' is invalid", o.ExternalConfig.HTTPProxy)
		}
		if err = checkNetConnectivity(o.ExternalConfig.HTTPProxy); err != nil {
			return errors.Wrapf(err, "check http_proxy connectivity failed")
		}
		blog.Infof("set http_proxy '%s' success", o.ExternalConfig.HTTPProxy)
	}
	return nil
}

// checkNetConnectivity check whether the target can connect
func checkNetConnectivity(target string) error {
	afterTrim := strings.TrimPrefix(strings.TrimPrefix(target, "http://"), "https://")
	conn, err := net.DialTimeout("tcp", afterTrim, 5*time.Second)
	if err != nil {
		return errors.Wrapf(err, "dial target '%s' failed", target)
	}
	defer conn.Close()
	return nil
}
