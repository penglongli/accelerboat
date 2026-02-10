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

func NewSetNamespaceCmd() *cobra.Command {
	var ns string
	cmd := &cobra.Command{
		Use:   "set-namespace [namespace]",
		Short: "Set default namespace in config file",
		Long:  "Saves the default namespace to the CLI config file. Global -n/--namespace still overrides at runtime.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if ns == "" && len(args) > 0 {
				ns = args[0]
			}
			if ns == "" {
				return fmt.Errorf("namespace is required (e.g. accelerboat set-namespace accelerboat)")
			}
			cfg, err := config.Load(configFilePath)
			if err != nil {
				return err
			}
			cfg.Namespace = ns
			if err := cfg.Save(configFilePath); err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "namespace set to %s (saved to %s)\n", ns, configFilePath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "Default namespace")
	return cmd
}
