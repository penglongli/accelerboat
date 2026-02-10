// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/penglongli/accelerboat/cmd/cli/kube"
)

const (
	imagePreloadLabelApp  = "image-preload"
	imagePreloadTaskLabel = "accelerboat.github.com/image-preload-task"
)

func NewImagePreloadCmd() *cobra.Command {
	var (
		namespace   string
		images      string
		pullSecrets string
		nodes       string
	)
	cmd := &cobra.Command{
		Use:   "image-preload",
		Short: "Preload container images on cluster nodes by running one-off Jobs",
		Long:  "Creates Jobs that pull the given images on each target node, then watches until completion and cleans up.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImagePreload(cmd, namespace, images, pullSecrets, nodes)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace for preload Jobs (default: accelerboat or value from -n/config)")
	cmd.Flags().StringVar(&images, "images", "", "Comma-separated list of images to preload (required)")
	cmd.Flags().StringVar(&pullSecrets, "pullsecrets", "", "Comma-separated list of image pull secret names (optional)")
	cmd.Flags().StringVar(&nodes, "nodes", "", "Comma-separated list of node names to preload on (optional; default: all nodes)")
	return cmd
}

// NewImagePreloadCleanCmd returns the command that removes all image-preload Jobs and Pods in a namespace.
func NewImagePreloadCleanCmd() *cobra.Command {
	var namespace string
	cmd := &cobra.Command{
		Use:   "image-preload-clean",
		Short: "Remove all image-preload Jobs and Pods in a namespace",
		Long:  "Deletes all Jobs and Pods labeled with app=image-preload in the given namespace (e.g. leftover preload resources).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImagePreloadClean(namespace)
		},
	}
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace to clean (default: accelerboat or value from -n/config)")
	return cmd
}

func runImagePreloadClean(namespace string) error {
	ctx := context.Background()
	kubeconfig := effectiveKubeconfig()
	ns := namespace
	if ns == "" {
		ns = effectiveNamespace()
	}
	if ns == "" {
		ns = defaultNamespace
	}

	client, err := kube.NewClient(kubeconfig, ns)
	if err != nil {
		return err
	}

	selector := "app=" + imagePreloadLabelApp

	// Delete all image-preload Jobs (cascading delete removes their Pods)
	jobList, err := client.ListJobs(ctx, selector)
	if err != nil {
		return fmt.Errorf("list preload jobs: %w", err)
	}
	for i := range jobList.Items {
		name := jobList.Items[i].Name
		if err := client.DeleteJob(ctx, name); err != nil && !errors.IsNotFound(err) {
			fmt.Fprintf(os.Stderr, "Warning: delete job %s: %v\n", name, err)
		} else {
			fmt.Fprintf(os.Stdout, "Deleted Job %s\n", name)
		}
	}

	// Delete any remaining Pods with the preload label (e.g. orphans)
	podList, err := client.ListPodsBySelector(ctx, selector)
	if err != nil {
		return fmt.Errorf("list preload pods: %w", err)
	}
	for i := range podList.Items {
		name := podList.Items[i].Name
		if err := client.DeletePod(ctx, name); err != nil && !errors.IsNotFound(err) {
			fmt.Fprintf(os.Stderr, "Warning: delete pod %s: %v\n", name, err)
		} else {
			fmt.Fprintf(os.Stdout, "Deleted Pod %s\n", name)
		}
	}

	if len(jobList.Items) == 0 && len(podList.Items) == 0 {
		fmt.Fprintf(os.Stdout, "No image-preload Jobs or Pods found in namespace %s.\n", ns)
	} else {
		fmt.Fprintf(os.Stdout, "Cleaned namespace %s: %d Job(s), %d Pod(s) removed.\n", ns, len(jobList.Items), len(podList.Items))
	}
	return nil
}

func runImagePreload(cmd *cobra.Command, namespace, imagesStr, pullSecretsStr, nodesStr string) error {
	imagesList := parseCommaList(imagesStr)
	if len(imagesList) == 0 {
		return fmt.Errorf("--images is required and must contain at least one image")
	}

	ctx := context.Background()
	kubeconfig := effectiveKubeconfig()
	ns := namespace
	if ns == "" {
		ns = effectiveNamespace()
	}
	if ns == "" {
		ns = defaultNamespace
	}

	client, err := kube.NewClient(kubeconfig, ns)
	if err != nil {
		return err
	}

	nodeNames, err := resolveNodeNames(ctx, client, nodesStr)
	if err != nil {
		return err
	}
	if len(nodeNames) == 0 {
		return fmt.Errorf("no target nodes found")
	}

	taskName := fmt.Sprintf("image-preload-%d", time.Now().UnixMilli())
	pullSecretNames := parseCommaList(pullSecretsStr)

	fmt.Fprintf(os.Stdout, "Image preload task: %s\n", taskName)
	fmt.Fprintf(os.Stdout, "Namespace: %s | Images: %v | Nodes: %d\n\n", ns, imagesList, len(nodeNames))

	createdJobs, err := createPreloadJobs(ctx, client, taskName, imagesList, pullSecretNames, nodeNames)
	if err != nil {
		return err
	}

	// Watch and report status until all jobs complete (success or failure)
	if err := watchPreloadJobs(ctx, client, taskName, createdJobs); err != nil {
		cleanupPreloadJobs(ctx, client, taskName)
		return err
	}

	// Cleanup
	fmt.Fprintln(os.Stdout, "\nCleaning up Jobs...")
	return cleanupPreloadJobs(ctx, client, taskName)
}

func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// sanitizeJobName makes a string safe for use in a Job name (DNS subdomain, max 63 chars).
func sanitizeJobName(name string, maxLen int) string {
	// Allow only [a-z0-9-], replace others with '-'
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	name = strings.ToLower(name)
	name = re.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if len(name) > maxLen {
		name = name[:maxLen]
	}
	return name
}

func resolveNodeNames(ctx context.Context, client *kube.Client, nodesStr string) ([]string, error) {
	if nodesStr != "" {
		return parseCommaList(nodesStr), nil
	}
	// All nodes: use worker nodes only (exclude master/control-plane)
	return client.ListWorkerNodeNames(ctx)
}

func createPreloadJobs(ctx context.Context, client *kube.Client, taskName string, images []string, pullSecretNames,
	nodeNames []string) ([]string, error) {
	var created []string
	for _, nodeName := range nodeNames {
		jobName := buildJobName(taskName, nodeName)
		job := buildPreloadJob(jobName, taskName, nodeName, images, pullSecretNames)
		_, err := client.CreateJob(ctx, job)
		if err != nil {
			return created, fmt.Errorf("create job %s: %w", jobName, err)
		}
		created = append(created, jobName)
		fmt.Fprintf(os.Stdout, "Created Job %s on node %s\n", jobName, nodeName)
	}
	return created, nil
}

func buildJobName(taskName, nodeName string) string {
	// Job name must be DNS subdomain, max 63 chars. taskName is e.g. image-preload-1739184000000
	prefix := taskName + "-"
	maxNodeLen := 63 - len(prefix)
	safeNode := sanitizeJobName(nodeName, maxNodeLen)
	if safeNode == "" {
		safeNode = "node"
	}
	return prefix + safeNode
}

func buildPreloadJob(jobName, taskName, nodeName string, images, pullSecretNames []string) *batchv1.Job {
	containers := make([]corev1.Container, 0, len(images))
	for i, image := range images {
		containers = append(containers, corev1.Container{
			Name:    fmt.Sprintf("preload-%d", i),
			Image:   image,
			Command: []string{"sh", "-c"},
			Args:    []string{`echo "✅ Image prewarmed on node $(hostname)"; exit 0`},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10m"),
					corev1.ResourceMemory: resource.MustParse("10Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10m"),
					corev1.ResourceMemory: resource.MustParse("10Mi"),
				},
			},
		})
	}

	var imagePullSecrets []corev1.LocalObjectReference
	for _, name := range pullSecretNames {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: name})
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   jobName,
			Labels: map[string]string{"app": imagePreloadLabelApp, imagePreloadTaskLabel: taskName},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr(int32(3)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": imagePreloadLabelApp, imagePreloadTaskLabel: taskName},
				},
				Spec: corev1.PodSpec{
					NodeName:         nodeName,
					RestartPolicy:    corev1.RestartPolicyOnFailure,
					Containers:       containers,
					ImagePullSecrets: imagePullSecrets,
				},
			},
		},
	}
	return job
}

func ptr(i int32) *int32 { return &i }

func watchPreloadJobs(ctx context.Context, client *kube.Client, taskName string, jobNames []string) error {
	selector := fmt.Sprintf("app=%s,%s=%s", imagePreloadLabelApp, imagePreloadTaskLabel, taskName)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Track which jobs have had their terminal-state logged to avoid duplicate output
	loggedTerminal := make(map[string]bool)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		list, err := client.ListJobs(ctx, selector)
		if err != nil {
			return fmt.Errorf("list jobs: %w", err)
		}

		var succeeded, active, failedCount int
		var failedNames []string
		for i := range list.Items {
			j := &list.Items[i]
			nodeName := j.Spec.Template.Spec.NodeName
			if nodeName == "" {
				nodeName = "<unknown>"
			}
			if j.Status.Active > 0 {
				active++
			} else if j.Status.Failed > 0 {
				failedCount++
				failedNames = append(failedNames, j.Name)
				if !loggedTerminal[j.Name] {
					loggedTerminal[j.Name] = true
					elapsed := jobElapsed(j)
					fmt.Fprintf(os.Stdout, "[terminal] Job %s | node: %s | status: Failed | duration: %s\n", j.Name, nodeName, elapsed)
				}
			} else {
				succeeded++
				if !loggedTerminal[j.Name] {
					loggedTerminal[j.Name] = true
					elapsed := jobElapsed(j)
					fmt.Fprintf(os.Stdout, "[terminal] Job %s | node: %s | status: Succeeded | duration: %s\n", j.Name, nodeName, elapsed)
				}
			}
		}

		total := len(list.Items)
		fmt.Fprintf(os.Stdout, "[%s] Jobs: %d total, %d succeeded, %d active, %d failed\n",
			time.Now().Format("15:04:05"), total, succeeded, active, failedCount)

		if active == 0 {
			if failedCount > 0 {
				fmt.Fprintf(os.Stdout, "Failed Jobs: %s\n", strings.Join(failedNames, ", "))
				return fmt.Errorf("one or more Jobs failed: %v", failedNames)
			}
			fmt.Fprintln(os.Stdout, "All preload Jobs completed successfully.")
			return nil
		}
	}
}

func jobElapsed(j *batchv1.Job) time.Duration {
	start := j.CreationTimestamp.Time
	end := time.Now()
	if j.Status.CompletionTime != nil {
		end = j.Status.CompletionTime.Time
	} else if j.Status.StartTime != nil {
		start = j.Status.StartTime.Time
	}
	return end.Sub(start).Round(time.Second)
}

func cleanupPreloadJobs(ctx context.Context, client *kube.Client, taskName string) error {
	selector := fmt.Sprintf("app=%s,%s=%s", imagePreloadLabelApp, imagePreloadTaskLabel, taskName)
	list, err := client.ListJobs(ctx, selector)
	if err != nil {
		return fmt.Errorf("list jobs for cleanup: %w", err)
	}
	for i := range list.Items {
		name := list.Items[i].Name
		if err := client.DeleteJob(ctx, name); err != nil && !errors.IsNotFound(err) {
			fmt.Fprintf(os.Stderr, "Warning: delete job %s: %v\n", name, err)
		} else {
			fmt.Fprintf(os.Stdout, "Deleted Job %s\n", name)
		}
	}
	return nil
}
