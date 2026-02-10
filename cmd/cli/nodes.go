// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"

	"github.com/penglongli/accelerboat/cmd/cli/kube"
)

func NewNodesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nodes",
		Short: "List AccelerBoat pods (like kubectl get pods -l app.kubernetes.io/name=accelerboat -o wide)",
		RunE:  runNodes,
	}
	return cmd
}

func runNodes(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := kube.NewClient(effectiveKubeconfig(), effectiveNamespace())
	if err != nil {
		return err
	}
	list, err := client.ListPods(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tREADY\tSTATUS\tRESTARTS\tAGE\tIP\tNODE")
	for i := range list.Items {
		p := &list.Items[i]
		ready := fmt.Sprintf("%d/%d", podReadyCount(p), len(p.Spec.Containers))
		age := "?"
		if !p.CreationTimestamp.IsZero() {
			age = durationShort(time.Since(p.CreationTimestamp.Time))
		}
		ip := p.Status.PodIP
		node := p.Spec.NodeName
		if node == "" {
			node = "<none>"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			p.Name,
			ready,
			string(p.Status.Phase),
			podRestartCount(p),
			age,
			ip,
			node,
		)
	}
	return tw.Flush()
}

func podReadyCount(p *corev1.Pod) int {
	var n int
	for _, c := range p.Status.ContainerStatuses {
		if c.Ready {
			n++
		}
	}
	return n
}

func podRestartCount(p *corev1.Pod) int {
	var n int32
	for _, c := range p.Status.ContainerStatuses {
		n += c.RestartCount
	}
	return int(n)
}

func durationShort(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
