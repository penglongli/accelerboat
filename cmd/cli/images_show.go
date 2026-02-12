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

const customapiOCIImages = "/customapi/oci-images"

func NewImagesShowCmd() *cobra.Command {
	var (
		instanceOrNode string
		outputFormat   string
	)
	cmd := &cobra.Command{
		Use:   "images-show",
		Short: "Get OCI images and layer digests via /customapi/oci-images",
		Long:  "Calls GET /customapi/oci-images on the target pod and prints image and digest info. Use -i to specify the pod name (e.g. accelerboat-5bj67).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if instanceOrNode == "" {
				return fmt.Errorf("--instance (-i) is required: pass the pod name (e.g. accelerboat-5bj67)")
			}
			ctx := context.Background()
			client, err := kube.NewClient(effectiveKubeconfig(), effectiveNamespace())
			if err != nil {
				return err
			}
			pod, err := client.GetPod(ctx, instanceOrNode)
			if err != nil {
				return err
			}
			query := url.Values{}
			if outputFormat == "json" {
				query.Set("output", "json")
			}
			body, err := client.PortForwardAndRequest(ctx, pod.Name, kube.HTTPPortNumber, customapiOCIImages, query)
			if err != nil {
				return err
			}
			_, _ = os.Stdout.Write(body)
			return nil
		},
	}
	cmd.Flags().StringVarP(&instanceOrNode, "instance", "i", "", "Pod name to query (required)")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format: json (default: human-readable text)")
	return cmd
}
