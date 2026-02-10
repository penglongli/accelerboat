// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/penglongli/accelerboat/cmd/cli/kube"
)

const (
	customapiRecorder = "/customapi/recorder"
	defaultTail       = 300
)

func NewEventsCmd() *cobra.Command {
	var (
		instanceID   string
		outputFormat string
		follow       bool
		tail         int
		registry     string
		search       string
	)
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Get recorder events from an instance via port-forward",
		RunE: func(cmd *cobra.Command, args []string) error {
			if instanceID == "" {
				return fmt.Errorf("--instance-id (-i) is required")
			}
			ctx := context.Background()
			if follow {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				go func() {
					<-sigCh
					cancel()
				}()
				defer cancel()
			}
			client, err := kube.NewClient(effectiveKubeconfig(), effectiveNamespace())
			if err != nil {
				return err
			}
			pod, err := client.GetPod(ctx, instanceID)
			if err != nil {
				return err
			}
			query := url.Values{}
			query.Set("limit", strconv.Itoa(tail))
			if outputFormat == "json" {
				query.Set("output", "json")
			}
			if follow {
				query.Set("follow", "true")
			}
			if registry != "" {
				query.Set("registry", registry)
			}
			if search != "" {
				query.Set("search", search)
			}
			if follow {
				return client.PortForwardAndStream(ctx, pod.Name, kube.HTTPPortNumber, customapiRecorder, query, os.Stdout)
			}
			body, err := client.PortForwardAndRequest(ctx, pod.Name, kube.HTTPPortNumber, customapiRecorder, query)
			if err != nil {
				return err
			}
			_, _ = os.Stdout.Write(body)
			return nil
		},
	}
	cmd.Flags().StringVarP(&instanceID, "instance-id", "i", "", "Instance (pod) ID (required)")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format: json")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream events (like kubectl logs -f)")
	cmd.Flags().IntVar(&tail, "tail", defaultTail, "Number of recent events to fetch")
	cmd.Flags().StringVar(&registry, "registry", "", "Filter by registry (exact match)")
	cmd.Flags().StringVar(&search, "search", "", "Filter by substring match on repo/extra")
	return cmd
}
