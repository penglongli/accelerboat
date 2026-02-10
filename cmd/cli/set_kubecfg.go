// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/penglongli/accelerboat/cmd/cli/config"
)

func NewSetKubecfgCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "set-kubecfg [path]",
		Short: "Set kubeconfig file path in config file",
		Long:  "Saves the kubeconfig path to the CLI config file. Global --kubeconfig still overrides at runtime.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if path == "" && len(args) > 0 {
				path = args[0]
			}
			if path == "" {
				return fmt.Errorf("kubeconfig path is required (e.g. accelerboat set-kubecfg ~/.kube/config)")
			}
			cfg, err := config.Load(configFilePath)
			if err != nil {
				return err
			}
			cfg.Kubeconfig = path
			if err := cfg.Save(configFilePath); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "kubeconfig set to %s (saved to %s)\n", path, configFilePath)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Path to kubeconfig file")
	return cmd
}
