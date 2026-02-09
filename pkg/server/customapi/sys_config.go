// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
)

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
	Address          string                   `json:"address"`
	HTTPPort         int64                    `json:"httpPort"`
	HTTPSPort        int64                    `json:"httpsPort"`
	MetricPort       int64                    `json:"metricPort"`
	TorrentPort      int64                    `json:"torrentPort"`
	LogConfig        options.LogConfig        `json:"logConfig"`
	StorageConfig    options.StorageConfig    `json:"storageConfig"`
	CleanConfig      options.CleanConfig      `json:"cleanConfig"`
	ServiceDiscovery options.ServiceDiscovery `json:"serviceDiscovery"`
	EnableContainerd bool                     `json:"enableContainerd"`
	TorrentConfig    options.TorrentConfig    `json:"torrentConfig"`
	RedisAddress     string                   `json:"redisAddress"`
	RedisPassword    string                   `json:"redisPassword"` // 注意：生产环境建议脱敏
	ExternalConfig   externalConfigSnapshot   `json:"externalConfig"`
}

type externalConfigSnapshot struct {
	HTTPProxy         string                    `json:"httpProxy"`
	DockerHubRegistry registryMappingSnapshot   `json:"dockerHubRegistry"`
	RegistryMappings  []registryMappingSnapshot `json:"registryMappings"`
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
		HTTPPort:         op.HTTPPort,
		HTTPSPort:        op.HTTPSPort,
		TorrentPort:      op.TorrentPort,
		LogConfig:        op.LogConfig,
		StorageConfig:    op.StorageConfig,
		CleanConfig:      op.CleanConfig,
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
