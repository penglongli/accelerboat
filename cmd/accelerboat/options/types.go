// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"net/url"

	"k8s.io/client-go/kubernetes"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options/leaderselector"
)

// AccelerBoatOption defines the option of accelerboat
type AccelerBoatOption struct {
	Address     string `json:"address"`
	HTTPPort    int64  `json:"httpPort"`
	HTTPSPort   int64  `json:"httpsPort"`
	MetricPort  int64  `json:"metricPort"`
	TorrentPort int64  `json:"torrentPort"`

	// LogConfig defines the log config
	LogConfig LogConfig `json:"logConfig"`
	// StorageConfig defines the paths that accelerboat will use
	StorageConfig StorageConfig `json:"storageConfig"`
	// CleanConfig Configure cleanup policies, allowing users to configure cleanup time,
	// disk usage thresholds, and how many days of data to retain
	CleanConfig CleanConfig `json:"cleanConfig" usage:"clean config"`

	// ServiceDiscovery defines the discovery between all nodes
	ServiceDiscovery ServiceDiscovery `json:"serviceDiscovery"`

	// EnableContainerd enable containerd image discovery
	EnableContainerd bool `json:"enableContainerd"`

	// TorrentConfig defines the config for torrent
	TorrentConfig TorrentConfig `json:"torrentConfig"`

	// Redis used to save some cache
	RedisAddress  string `json:"redisAddress"`
	RedisPassword string `json:"redisPassword"`

	// ExternalConfig defines the external config
	ExternalConfig ExternalConfig `json:"externalConfig"`

	k8sClient *kubernetes.Clientset
}

// LogConfig defines the config of log
type LogConfig struct {
	LogDir        string `json:"logDir"`
	LogMaxSize    int    `json:"logMaxSize"`
	LogMaxBackups int    `json:"logMaxBackups"`
	LogMaxAge     int    `json:"logMaxAge"`
}

// StorageConfig defines the config of storage
type StorageConfig struct {
	// DownloadPath storage directory for Layers downloaded from the source registry.
	// The integrity of the files under it cannot be guaranteed.
	DownloadPath string `json:"downloadPath"`
	// TorrentPath Directory for Torrent downloads, the integrity of the files is not guaranteed
	TorrentPath string `json:"torrentPath"`
	// TransferPath layer file is stored for regular downloads, and the files below
	// it are guaranteed to be complete
	TransferPath string `json:"transferPath"`
	// SmallFilePath Small file, the integrity of the files below is guaranteed
	SmallFilePath string `json:"smallFilePath"`
	// OCIPath Stores files cached by the Layer managed by containerd to ensure integrity
	OCIPath string `json:"ociPath"`
	// EventFile defines the file to store events
	EventFile string `json:"eventFile"`
}

// TorrentConfig defines the config of torrent
type TorrentConfig struct {
	// Enable whether enable torrent file transfer
	Enable bool `json:"enable"`
	// Threshold the threshold for Torrent file transfer. Torrent transfer is used
	// only when the threshold is exceeded.
	Threshold int64 `json:"threshold"`
	// UploadLimit upload speed limit for torrent seeds. 0 means no limit.
	UploadLimit int64 `json:"uploadLimit"`
	// DownloadLimit download speed limit for torrent seeds. 0 means no limit.
	DownloadLimit int64 `json:"downloadLimit"`
	// Announce defines the announce address for torrent
	Announce string `json:"announce"`
}

// ProxyKeyCert defines the key/cert for proxy host
type ProxyKeyCert struct {
	Key  string `json:"key"`
	Cert string `json:"cert"`
}

// RegistryAuth defines the user/pass for registry
type RegistryAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegistryMapping defines the mapping for original registry with proxy. There also defines the
// username/password for registry when use RegistryMirror mode.
type RegistryMapping struct {
	Enable       bool   `json:"enable"`
	ProxyHost    string `json:"proxyHost"`
	ProxyCert    string `json:"proxyCert"`
	ProxyKey     string `json:"proxyKey"`
	OriginalHost string `json:"originalHost"`

	Username string          `json:"username"`
	Password string          `json:"password"`
	Users    []*RegistryAuth `json:"users,omitempty"`
	// temporary store the legal auths
	LegalUsers []*RegistryAuth `json:"-"`
}

// LocalhostCert defines localhost proxy
const LocalhostCert = "localhost"

// ExternalConfig defines the external config
type ExternalConfig struct {
	HTTPProxy         string                   `json:"httpProxy"`
	HTTPProxyUrl      *url.URL                 `json:"-"`
	BuiltInCerts      map[string]*ProxyKeyCert `json:"builtInCerts"`
	DockerHubRegistry RegistryMapping          `json:"dockerHubRegistry"`
	RegistryMappings  []*RegistryMapping       `json:"registryMappings"`
}

type ServiceDiscovery struct {
	ServiceNamespace string `json:"serviceNamespace"`
	ServiceName      string `json:"serviceName"`

	// PreferConfig with the priority configuration strategy, users can specify the Master node
	// and designate certain nodes as preferred roles.
	PreferConfig leaderselector.PreferConfig `json:"preferConfig" value:"" usage:"prefer config"`
}

// CleanConfig defines the clean config
type CleanConfig struct {
	Cron       string `json:"cron" usage:"the cron expression"`
	Threshold  int64  `json:"threshold"`
	RetainDays int64  `json:"retainDays"`
}
