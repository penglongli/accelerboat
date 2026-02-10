// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath returns the default config file path (~/.accelerboat.yaml).
func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".accelerboat.yaml"
	}
	return filepath.Join(home, ".accelerboat.yaml")
}

// ExpandPath expands ~ to user home directory.
func ExpandPath(path string) string {
	if path == "" || len(path) < 2 || path[0] != '~' {
		return path
	}
	if path[1] == '/' || path[1] == filepath.Separator {
		home, _ := os.UserHomeDir()
		if home == "" {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// Config holds CLI configuration persisted to file.
type Config struct {
	Kubeconfig string `yaml:"kubeconfig,omitempty"`
	Namespace  string `yaml:"namespace,omitempty"`
}

// Load reads config from path. Returns nil config if file does not exist or is invalid.
func Load(path string) (*Config, error) {
	path = ExpandPath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save writes config to path.
func (c *Config) Save(path string) error {
	path = ExpandPath(path)
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
