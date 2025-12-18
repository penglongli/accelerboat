// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

// AccelerBoatOption defines the option of accelerboat
type AccelerBoatOption struct {
	Address     string `json:"address"`
	HTTPPort    int64  `json:"httpPort"`
	HTTPSPort   int64  `json:"httpsPort"`
	MetricPort  int64  `json:"metricPort"`
	TorrentPort int64  `json:"torrentPort"`

	LogDir        string `json:"logDir"`
	LogMaxSize    int    `json:"logMaxSize"`
	LogMaxBackups int    `json:"logMaxBackups"`
	LogMaxAge     int    `json:"logMaxAge"`

	// DisableTorrent disable torrent file transfer
	DisableTorrent bool `json:"disableTorrent"`
	// EnableContainerd enable containerd image discovery
	EnableContainerd bool `json:"enableContainerd"`

	// TorrentThreshold the threshold for Torrent file transfer. Torrent transfer is used
	// only when the threshold is exceeded.
	TorrentThreshold int64 `json:"torrentThreshold"`
	// TorrentUploadLimit upload speed limit for torrent seeds. 0 means no limit.
	TorrentUploadLimit int64 `json:"torrentUploadLimit"`
	// TorrentDownloadLimit download speed limit for torrent seeds. 0 means no limit.
	TorrentDownloadLimit int64 `json:"torrentDownloadLimit"`
	// TorrentAnnounce defines the announce address for torrent
	TorrentAnnounce string `json:"torrentAnnounce" usage:"the announce of torrent"`

	// StoragePath storage directory for Layers downloaded from the source repository.
	// The integrity of the files under it cannot be guaranteed.
	StoragePath string `json:"storagePath"`
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

	// PreferConfig 配置优先策略，用户可以指定 Master，以及指定某些节点作为优选的角色
	PreferConfig PreferConfig `json:"preferConfig" value:"" usage:"prefer config"`

	// CleanConfig Configure cleanup policies, allowing users to configure cleanup time,
	// disk usage thresholds, and how many days of data to retain
	CleanConfig CleanConfig `json:"cleanConfig" usage:"clean config"`

	// Redis used to save some cache
	RedisAddress  string `json:"redisAddress"`
	RedisPassword string `json:"redisPassword"`

	// ServiceDiscovery defines the discovery between all nodes
	ServiceDiscovery *ServiceDiscovery `json:"serviceDiscovery"`

	// ExternalConfig defines the external config
	ExternalConfig ExternalConfig `json:"externalConfig"`
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
	ProxyHost    string `json:"proxyHost"`
	ProxyCert    string `json:"proxyCert"`
	ProxyKey     string `json:"proxyKey"`
	OriginalHost string `json:"originalHost"`

	Username string          `json:"username"`
	Password string          `json:"password"`
	Users    []*RegistryAuth `json:"users,omitempty"`
	// 用户多个用户名/密码，临时记录正确的内容
	CorrectUser string `json:"-"`
	CorrectPass string `json:"-"`
}

// LocalhostCert defines localhost proxy
const LocalhostCert = "localhost"

// ExternalConfig defines the external config
type ExternalConfig struct {
	Enable            bool                     `json:"enable"`
	HTTPProxy         string                   `json:"httpProxy"`
	BuiltInCerts      map[string]*ProxyKeyCert `json:"builtInCerts"`
	DockerHubRegistry RegistryMapping          `json:"dockerHubRegistry"`
	RegistryMappings  []*RegistryMapping       `json:"registryMappings"`
}

type ServiceDiscovery struct {
	ServiceNamespace string   `json:"serviceNamespace"`
	ServiceName      string   `json:"serviceName"`
	Endpoints        []string `json:"-"`
}

// PreferConfig defines the prefer config
type PreferConfig struct {
	MasterIP    string            `json:"masterIP" value:"" usage:"manually specify the master node"`
	PreferNodes PreferNodesConfig `json:"preferNodes" value:"" usage:"assume the master role and download tasks"`
}

// CleanConfig defines the clean config
type CleanConfig struct {
	Cron       string `json:"cron" usage:"the cron expression"`
	Threshold  int64  `json:"threshold"`
	RetainDays int64  `json:"retainDays"`
}

// PreferNodesConfig defines the prefer nodes config
type PreferNodesConfig struct {
	LabelSelectors string `json:"labelSelectors" usage:"the label selector to filter nodes"`
}

// ProxyType defines proxy type
type ProxyType string

const (
	// DomainProxy domain proxy
	DomainProxy ProxyType = "DomainProxy"
	// RegistryMirror registry mirror
	RegistryMirror ProxyType = "RegistryMirror"
)

var (
	singleton = new(AccelerBoatOption)
)

// GlobalOptions returns the global option
func GlobalOptions() *AccelerBoatOption {
	return singleton
}

// FilterRegistryMapping filter registry mapping
func (o *AccelerBoatOption) FilterRegistryMapping(proxyHost string, proxyType ProxyType) *RegistryMapping {
	// 针对 ProxyHost 为空，设置其默认使用 docker.io
	if proxyHost == "" {
		return &o.ExternalConfig.DockerHubRegistry
	}
	for _, m := range o.ExternalConfig.RegistryMappings {
		switch proxyType {
		case RegistryMirror:
			// for containerd
			if proxyHost == m.OriginalHost {
				return m
			}
			// for dockerd
			if proxyHost == m.ProxyHost {
				return m
			}
		case DomainProxy:
			if proxyHost == m.ProxyHost {
				return m
			}
		}
	}
	return nil
}
