// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/cmd/accelerboat/options/leaderselector"
)

// --- Stats ---

type statsJSON struct {
	ContainerdEnabled bool              `json:"containerdEnabled"`
	Torrent           torrentStatsJSON  `json:"torrent"`
	Master            string            `json:"master"`
	HTTPProxy         string            `json:"httpProxy"`
	Upstreams         []upstreamEntryJSON `json:"upstreams"`
}

type torrentStatsJSON struct {
	Enabled       bool   `json:"enabled"`
	Threshold     int64  `json:"threshold"`
	UploadLimit   int64  `json:"uploadLimit"`
	DownloadLimit int64  `json:"downloadLimit"`
	Announce      string `json:"announce"`
}

type upstreamEntryJSON struct {
	ProxyHost    string `json:"proxyHost"`
	OriginalHost string `json:"originalHost"`
	Enabled      bool   `json:"enabled"`
}

func (h *CustomHandler) Stats(c *gin.Context) (interface{}, string, error) {
	op := h.op
	tc := op.TorrentConfig
	js := statsJSON{
		ContainerdEnabled: op.EnableContainerd,
		Torrent: torrentStatsJSON{
			Enabled:       tc.Enable,
			Threshold:     tc.Threshold,
			UploadLimit:   tc.UploadLimit,
			DownloadLimit: tc.DownloadLimit,
			Announce:      tc.Announce,
		},
		Master:    leaderselector.CurrentMaster(),
		HTTPProxy: op.ExternalConfig.HTTPProxy,
		Upstreams: buildUpstreamsList(op),
	}
	text := formatStats(js)
	return js, text, nil
}

func buildUpstreamsList(op *options.AccelerBoatOption) []upstreamEntryJSON {
	list := make([]upstreamEntryJSON, 0, 1+len(op.ExternalConfig.RegistryMappings))
	dh := op.ExternalConfig.DockerHubRegistry
	proxyHost := dh.ProxyHost
	if proxyHost == "" {
		proxyHost = "docker.io"
	}
	list = append(list, upstreamEntryJSON{
		ProxyHost:    proxyHost,
		OriginalHost: dh.OriginalHost,
		Enabled:      dh.Enable,
	})
	for _, m := range op.ExternalConfig.RegistryMappings {
		list = append(list, upstreamEntryJSON{
			ProxyHost:    m.ProxyHost,
			OriginalHost: m.OriginalHost,
			Enabled:      m.Enable,
		})
	}
	return list
}

func formatStats(js statsJSON) string {
	var b strings.Builder
	b.WriteString("=== AccelerBoat Stats ===\n\n")
	b.WriteString(fmt.Sprintf("Containerd:    %s\n", formatBool(js.ContainerdEnabled)))
	b.WriteString(fmt.Sprintf("Torrent:       %s\n", formatBool(js.Torrent.Enabled)))
	if js.Torrent.Enabled {
		b.WriteString(fmt.Sprintf("  Threshold:     %d (MB)\n", js.Torrent.Threshold))
		b.WriteString(fmt.Sprintf("  UploadLimit:   %d (0=unlimited)\n", js.Torrent.UploadLimit))
		b.WriteString(fmt.Sprintf("  DownloadLimit: %d (0=unlimited)\n", js.Torrent.DownloadLimit))
		b.WriteString(fmt.Sprintf("  Announce:      %s\n", js.Torrent.Announce))
	}
	b.WriteString(fmt.Sprintf("Master:        %s\n", js.Master))
	b.WriteString(fmt.Sprintf("HTTPProxy:     %s\n", orEmpty(js.HTTPProxy)))
	b.WriteString("\nUpstreams:\n")
	for _, u := range js.Upstreams {
		b.WriteString(fmt.Sprintf("  - %s -> %s  [Enabled: %s]\n", u.ProxyHost, u.OriginalHost, formatBool(u.Enabled)))
	}
	return b.String()
}

func formatBool(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}

func orEmpty(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

// --- Metrics ---

func (h *CustomHandler) Metrics(c *gin.Context) (interface{}, string, error) {
	data, err := gatherMetricsJSON()
	if err != nil {
		return nil, "", err
	}
	text := formatMetrics(data)
	return data, text, nil
}

func gatherMetricsJSON() (map[string]interface{}, error) {
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}
	out := make(map[string]interface{})
	for _, mf := range metricFamilies {
		if mf.Name == nil {
			continue
		}
		name := *mf.Name
		entries := make([]map[string]interface{}, 0)
		for _, m := range mf.Metric {
			entry := make(map[string]interface{})
			if len(m.Label) > 0 {
				labels := make(map[string]string)
				for _, l := range m.Label {
					if l.Name != nil && l.Value != nil {
						labels[*l.Name] = *l.Value
					}
				}
				entry["labels"] = labels
			}
			if m.Counter != nil && m.Counter.Value != nil {
				entry["value"] = *m.Counter.Value
				entry["type"] = "counter"
			} else if m.Gauge != nil && m.Gauge.Value != nil {
				entry["value"] = *m.Gauge.Value
				entry["type"] = "gauge"
			} else if m.Untyped != nil && m.Untyped.Value != nil {
				entry["value"] = *m.Untyped.Value
				entry["type"] = "untyped"
			} else if m.Histogram != nil {
				entry["type"] = "histogram"
				entry["sample_count"] = m.Histogram.GetSampleCount()
				entry["sample_sum"] = m.Histogram.GetSampleSum()
			}
			if len(entry) > 0 {
				entries = append(entries, entry)
			}
		}
		if mf.Help != nil {
			out[name] = map[string]interface{}{
				"help":   *mf.Help,
				"metrics": entries,
			}
		} else {
			out[name] = map[string]interface{}{"metrics": entries}
		}
	}
	return out, nil
}

func formatMetrics(data map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("=== AccelerBoat Metrics ===\n\n")
	names := make([]string, 0, len(data))
	for k := range data {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		v := data[name].(map[string]interface{})
		b.WriteString(fmt.Sprintf("[%s]\n", name))
		if help, ok := v["help"].(string); ok && help != "" {
			b.WriteString(fmt.Sprintf("  %s\n", help))
		}
		metricsList, _ := v["metrics"].([]map[string]interface{})
		for _, m := range metricsList {
			if val, ok := m["value"]; ok {
				b.WriteString(fmt.Sprintf("  value=%v", val))
			}
			if labels, ok := m["labels"].(map[string]interface{}); ok && len(labels) > 0 {
				parts := make([]string, 0, len(labels))
				for k, v := range labels {
					parts = append(parts, fmt.Sprintf("%s=%v", k, v))
				}
				sort.Strings(parts)
				b.WriteString("  labels: " + strings.Join(parts, ", "))
			}
			if _, hasHist := m["sample_count"]; hasHist {
				b.WriteString(fmt.Sprintf("  count=%v sum=%v", m["sample_count"], m["sample_sum"]))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// --- Config ---

func (h *CustomHandler) Config(c *gin.Context) (interface{}, string, error) {
	op := h.op
	// 使用可导出的副本，避免循环引用；JSON 会序列化 json tag 的字段
	cfg := buildConfigSnapshot(op)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, "", err
	}
	var text strings.Builder
	text.WriteString("=== AccelerBoat Config ===\n\n")
	text.WriteString(string(raw))
	return cfg, text.String(), nil
}

// configSnapshot 用于 JSON/格式化输出，对敏感字段可做脱敏（此处保持与 options 一致，仅做快照）
type configSnapshot struct {
	Address       string                    `json:"address"`
	HTTPPort      int64                     `json:"httpPort"`
	HTTPSPort     int64                     `json:"httpsPort"`
	MetricPort    int64                     `json:"metricPort"`
	TorrentPort   int64                     `json:"torrentPort"`
	LogConfig     options.LogConfig         `json:"logConfig"`
	StorageConfig options.StorageConfig     `json:"storageConfig"`
	CleanConfig   options.CleanConfig       `json:"cleanConfig"`
	ServiceDiscovery options.ServiceDiscovery `json:"serviceDiscovery"`
	EnableContainerd bool                  `json:"enableContainerd"`
	TorrentConfig options.TorrentConfig    `json:"torrentConfig"`
	RedisAddress  string                   `json:"redisAddress"`
	RedisPassword string                   `json:"redisPassword"` // 注意：生产环境建议脱敏
	ExternalConfig externalConfigSnapshot  `json:"externalConfig"`
}

type externalConfigSnapshot struct {
	HTTPProxy        string                       `json:"httpProxy"`
	DockerHubRegistry registryMappingSnapshot    `json:"dockerHubRegistry"`
	RegistryMappings  []registryMappingSnapshot   `json:"registryMappings"`
}

type registryMappingSnapshot struct {
	Enable       bool   `json:"enable"`
	ProxyHost    string `json:"proxyHost"`
	OriginalHost string `json:"originalHost"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

func buildConfigSnapshot(op *options.AccelerBoatOption) configSnapshot {
	ext := op.ExternalConfig
	snap := configSnapshot{
		Address:          op.Address,
		HTTPPort:          op.HTTPPort,
		HTTPSPort:         op.HTTPSPort,
		MetricPort:        op.MetricPort,
		TorrentPort:       op.TorrentPort,
		LogConfig:         op.LogConfig,
		StorageConfig:    op.StorageConfig,
		CleanConfig:       op.CleanConfig,
		ServiceDiscovery: op.ServiceDiscovery,
		EnableContainerd: op.EnableContainerd,
		TorrentConfig:    op.TorrentConfig,
		RedisAddress:     op.RedisAddress,
		RedisPassword:    op.RedisPassword,
		ExternalConfig: externalConfigSnapshot{
			HTTPProxy: ext.HTTPProxy,
			DockerHubRegistry: registryMappingSnapshot{
				Enable:       ext.DockerHubRegistry.Enable,
				ProxyHost:    ext.DockerHubRegistry.ProxyHost,
				OriginalHost: ext.DockerHubRegistry.OriginalHost,
				Username:     ext.DockerHubRegistry.Username,
				Password:     ext.DockerHubRegistry.Password,
			},
			RegistryMappings: make([]registryMappingSnapshot, 0, len(ext.RegistryMappings)),
		},
	}
	for _, m := range ext.RegistryMappings {
		snap.ExternalConfig.RegistryMappings = append(snap.ExternalConfig.RegistryMappings, registryMappingSnapshot{
			Enable:       m.Enable,
			ProxyHost:    m.ProxyHost,
			OriginalHost: m.OriginalHost,
			Username:     m.Username,
			Password:     m.Password,
		})
	}
	return snap
}
