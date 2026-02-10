// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/penglongli/accelerboat/cmd/cli/kube"
)

const customapiStats = "/customapi/stats"

func NewStatsCmd() *cobra.Command {
	var (
		instanceID   string
		outputFormat string
	)
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Get stats from an instance via port-forward to /customapi/stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			if instanceID == "" {
				return fmt.Errorf("--instance-id (-i) is required")
			}
			ctx := context.Background()
			client, err := kube.NewClient(effectiveKubeconfig(), effectiveNamespace())
			if err != nil {
				return err
			}
			pod, err := client.GetPod(ctx, instanceID)
			if err != nil {
				return err
			}
			query := url.Values{}
			if outputFormat == "json" {
				query.Set("output", "json")
			}
			body, err := client.PortForwardAndRequest(ctx, pod.Name, kube.HTTPPortNumber, customapiStats, query)
			if err != nil {
				return err
			}
			_, _ = os.Stdout.Write(body)
			return nil
		},
	}
	cmd.Flags().StringVarP(&instanceID, "instance-id", "i", "", "Instance (pod) ID (required)")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format: json")
	return cmd
}
