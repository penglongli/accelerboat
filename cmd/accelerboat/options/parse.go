// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options/leaderselector"
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

func changeOption(op *AccelerBoatOption, init bool) {
	// initialized for the first time
	if init {
		_ = utils.DeepCopyStruct(op, singleton)
		_ = utils.DeepCopyStruct(op, prev)

		// only init for the first time
		disc := op.ServiceDiscovery
		if err := leaderselector.WatchK8sService(disc.ServiceNamespace, disc.ServiceName, op.HTTPPort,
			disc.PreferConfig, op.k8sClient); err != nil {
			logger.Fatalf("watch k8s service failed: %s", err)
		}
	} else {
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
	}
	logger.Infof("parsed options: %s", string(utils.ToJson(op)))
}

func Parse(configFile string, init bool) (*AccelerBoatOption, error) {
	bs, err := os.ReadFile(configFile)
	if err != nil {
		return nil, errors.Wrapf(err, "read config '%s' failed", configFile)
	}
	op := new(AccelerBoatOption)
	if err = json.Unmarshal(bs, op); err != nil {
		return nil, errors.Wrapf(err, "unmarshal config failed")
	}
	if init {
		logger.InitLogger(&logger.Option{
			Filename:   filepath.Join(op.LogConfig.LogDir, "accelerboat.log"),
			MaxSize:    op.LogConfig.LogMaxSize,
			MaxAge:     op.LogConfig.LogMaxAge,
			MaxBackups: op.LogConfig.LogMaxBackups,
		})
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
	if err = op.checkTorrentConfig(); err != nil {
		return nil, errors.Wrapf(err, "check option torrent config failed")
	}
	if err = op.checkExternalConfig(); err != nil {
		return nil, errors.Wrapf(err, "check option external config failed")
	}
	localIP := os.Getenv("localIP")
	if localIP == "" {
		return nil, fmt.Errorf("env 'localIP' is empty")
	}
	op.Address = localIP
	changeOption(op, init)
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
	// MB unit
	MB int64 = 1048576
	// TwentyMB 20MB
	TwentyMB int64 = 20 * MB
)

func (o *AccelerBoatOption) checkCleanConfig() error {
	if o.CleanConfig.Cron == "" {
		logger.Infof("clean-config not set, no-need auto clean")
		return nil
	}
	if o.CleanConfig.Threshold < 10 {
		o.CleanConfig.Threshold = 10
	}
	if o.CleanConfig.RetainDays < 0 {
		o.CleanConfig.RetainDays = 0
	}
	if err := ParseCron(o.CleanConfig.Cron); err != nil {
		return err
	}
	return nil
}

func ParseCron(expr string) error {
	parser := cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return errors.Wrapf(err, "parse cron expression '%s' failed", expr)
	}
	logger.Infof("parse clean cron '%s' success, print next execution times:", expr)
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
	if err != nil {
		return fmt.Errorf("get in-cluster config failed: %s", err.Error())
	}
	o.k8sClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Wrapf(err, "create kubernetes client failed")
	}
	if err = o.checkPreferConfig(); err != nil {
		return errors.Wrapf(err, "check option prefer config failed")
	}
	return nil
}

var (
	labelSelectorRegexp, _ = regexp.Compile(`^[^=,]+=[^=,]+(,[^=,]+=[^=,]+)*$`)
)

func (o *AccelerBoatOption) checkPreferConfig() error {
	selector := o.ServiceDiscovery.PreferConfig.PreferNodes.LabelSelectors
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
	// min 200 MB
	if o.TorrentConfig.Threshold < 200 {
		o.TorrentConfig.Threshold = 200
	}
	// min 10 MB
	if o.TorrentConfig.UploadLimit > 0 && o.TorrentConfig.UploadLimit < 10 {
		o.TorrentConfig.UploadLimit = 10
	}
	// min 10 MB
	if o.TorrentConfig.DownloadLimit > 0 && o.TorrentConfig.DownloadLimit < 10 {
		o.TorrentConfig.DownloadLimit = 10
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
		logger.Infof("set http_proxy '%s' success", o.ExternalConfig.HTTPProxy)
	}
	for _, v := range o.ExternalConfig.BuiltInCerts {
		keyBase64, err := base64.StdEncoding.DecodeString(v.Key)
		if err != nil {
			return errors.Wrapf(err, "base64 decode built-in key '%s' failed", v.Key)
		}

		var certBase64 []byte
		certBase64, err = base64.StdEncoding.DecodeString(v.Cert)
		if err != nil {
			return errors.Wrapf(err, "base64 decode built-in cert '%s' failed", v.Cert)
		}
		v.Key = string(keyBase64)
		v.Cert = string(certBase64)
	}
	for _, mp := range o.ExternalConfig.RegistryMappings {
		v, ok := o.ExternalConfig.BuiltInCerts[mp.ProxyHost]
		if ok {
			mp.ProxyCert = v.Cert
			mp.ProxyKey = v.Key
			continue
		}
		if mp.ProxyCert != "" && mp.ProxyKey != "" {
			afterBase64, err := base64.StdEncoding.DecodeString(mp.ProxyCert)
			if err != nil {
				return errors.Wrapf(err, "base64 decode cert '%s' failed", mp.ProxyCert)
			}
			mp.ProxyCert = string(afterBase64)
			afterBase64, err = base64.StdEncoding.DecodeString(mp.ProxyKey)
			if err != nil {
				return errors.Wrapf(err, "base64 decode key '%s' failed", mp.ProxyKey)
			}
			mp.ProxyKey = string(afterBase64)
		}
		if mp.Username != "" && mp.Password != "" {
			mp.LegalUsers = append(mp.LegalUsers, &RegistryAuth{
				Username: mp.Username,
				Password: mp.Password,
			})
		}
		for _, user := range mp.Users {
			if user.Username != "" && user.Password != "" {
				mp.LegalUsers = append(mp.LegalUsers, &RegistryAuth{
					Username: user.Username,
					Password: user.Password,
				})
			}
		}
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
