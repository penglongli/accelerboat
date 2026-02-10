// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/penglongli/accelerboat/cmd/cli/kube"
)

const (
	configMapName = "accelerboat-config"
	configMapKey = "accelerboat.json"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Get ConfigMap accelerboat-config accelerboat.json content in the namespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			client, err := kube.NewClient(effectiveKubeconfig(), effectiveNamespace())
			if err != nil {
				return err
			}
			cm, err := client.GetConfigMap(ctx, configMapName)
			if err != nil {
				return err
			}
			data, ok := cm.Data[configMapKey]
			if !ok {
				return fmt.Errorf("ConfigMap %s has no key %q", configMapName, configMapKey)
			}
			_, _ = os.Stdout.WriteString(data)
			return nil
		},
	}
	return cmd
}
