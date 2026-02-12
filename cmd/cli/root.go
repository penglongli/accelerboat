// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/penglongli/accelerboat/cmd/cli/config"
)

const (
	defaultNamespace  = "accelerboat"
	defaultKubeconfig = "~/.kube/config"
)

var (
	globalKubeconfig string
	globalNamespace  string
	configFilePath   string
)

func expandPath(p string) string {
	if len(p) == 0 || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[1:])
}

func effectiveKubeconfig() string {
	if globalKubeconfig != "" {
		return expandPath(globalKubeconfig)
	}
	cfg, _ := config.Load(configFilePath)
	if cfg != nil && cfg.Kubeconfig != "" {
		return expandPath(cfg.Kubeconfig)
	}
	return expandPath(defaultKubeconfig)
}

func effectiveNamespace() string {
	if globalNamespace != "" {
		return globalNamespace
	}
	cfg, _ := config.Load(configFilePath)
	if cfg != nil && cfg.Namespace != "" {
		return cfg.Namespace
	}
	return defaultNamespace
}

// NewRootCmd returns the root command with global flags and subcommands.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accelerboat",
		Short: "AccelerBoat CLI for Kubernetes",
		Long:  Logo(),
	}
	cmd.PersistentFlags().StringVar(&globalKubeconfig, "kubeconfig", "", "Path to kubeconfig file (default: ~/.kube/config or value from config file)")
	cmd.PersistentFlags().StringVarP(&globalNamespace, "namespace", "n", "", "Default namespace (default: accelerboat or value from config file)")
	cmd.PersistentFlags().StringVar(&configFilePath, "config-file", config.DefaultConfigPath(), "Path to accelerboat CLI config file")

	cmd.AddCommand(NewSetKubecfgCmd())
	cmd.AddCommand(NewSetNamespaceCmd())
	cmd.AddCommand(NewNodesCmd())
	cmd.AddCommand(NewStatsCmd())
	cmd.AddCommand(NewMetricsCmd())
	cmd.AddCommand(NewConfigCmd())
	cmd.AddCommand(NewEventsCmd())
	cmd.AddCommand(NewImagePreloadCmd())
	cmd.AddCommand(NewImagePreloadCleanCmd())
	cmd.AddCommand(NewImagesShowCmd())

	return cmd
}
