// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package kube

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	// AccelerboatAppLabel is the label selector for accelerboat pods.
	AccelerboatAppLabel = "app.kubernetes.io/name=accelerboat"
	// AccelerboatHTTPPort is the container port name for HTTP (values.env.httpPort 2080).
	AccelerboatHTTPPort = "http"
	// HTTPPortNumber is the default HTTP port in container.
	HTTPPortNumber = 2080
)

// Client wraps Kubernetes client and helpers for accelerboat CLI.
type Client struct {
	clientset *kubernetes.Clientset
	config    *restclient.Config
	namespace string
}

// NewClient builds a Kubernetes client from kubeconfig path and namespace.
func NewClient(kubeconfig, namespace string) (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}
	return &Client{
		clientset: clientset,
		config:    config,
		namespace: namespace,
	}, nil
}

// Namespace returns the configured namespace.
func (c *Client) Namespace() string {
	return c.namespace
}

// ListPods returns pods matching the accelerboat app label in the configured namespace.
func (c *Client) ListPods(ctx context.Context) (*corev1.PodList, error) {
	return c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: AccelerboatAppLabel,
	})
}

// GetConfigMap returns the named ConfigMap in the configured namespace.
func (c *Client) GetConfigMap(ctx context.Context, name string) (*corev1.ConfigMap, error) {
	return c.clientset.CoreV1().ConfigMaps(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

// PortForwardAndRequest runs a port-forward to the given pod/port, calls the given path with optional query,
// and returns the response body. The port-forward is stopped when ctx is cancelled.
func (c *Client) PortForwardAndRequest(ctx context.Context, podName string, port int, path string, query url.Values) ([]byte, error) {
	localPort, err := freeLocalPort()
	if err != nil {
		return nil, err
	}
	stopCh := make(chan struct{})
	defer close(stopCh)
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.portForward(ctx, podName, port, localPort, stopCh, readyCh)
	}()
	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-readyCh:
	}
	u := url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("127.0.0.1:%d", localPort),
		Path:     path,
		RawQuery: query.Encode(),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET %s: %s: %s", u.String(), resp.Status, string(body))
	}
	return io.ReadAll(resp.Body)
}

// PortForwardAndStream runs a port-forward and streams the response body to the given writer until context is done.
func (c *Client) PortForwardAndStream(ctx context.Context, podName string, port int, path string, query url.Values, w io.Writer) error {
	localPort, err := freeLocalPort()
	if err != nil {
		return err
	}
	stopCh := make(chan struct{})
	defer close(stopCh)
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.portForward(ctx, podName, port, localPort, stopCh, readyCh)
	}()
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-readyCh:
	}
	u := url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("127.0.0.1:%d", localPort),
		Path:     path,
		RawQuery: query.Encode(),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %s: %s", u.String(), resp.Status, string(body))
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

func (c *Client) portForward(ctx context.Context, podName string, remotePort, localPort int, stopCh chan struct{}, readyCh chan struct{}) error {
	roundTripper, upgrader, err := spdy.RoundTripperFor(c.config)
	if err != nil {
		return err
	}
	reqURL := c.clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(c.namespace).
		Name(podName).
		SubResource("portforward").URL()
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, reqURL)
	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	pf, err := portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return err
	}
	// Run in a goroutine so we can respect ctx
	done := make(chan error, 1)
	go func() {
		done <- pf.ForwardPorts()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// GetPod finds a pod by name (or unique prefix) in the accelerboat namespace. Name can be full pod name or instance id (e.g. accelerboat-0).
func (c *Client) GetPod(ctx context.Context, name string) (*corev1.Pod, error) {
	if name == "" {
		return nil, fmt.Errorf("pod name is required")
	}
	list, err := c.ListPods(ctx)
	if err != nil {
		return nil, err
	}
	var prefixMatch *corev1.Pod
	for i := range list.Items {
		p := &list.Items[i]
		if p.Name == name {
			return p, nil
		}
		if len(name) <= len(p.Name) && p.Name[:len(name)] == name {
			if prefixMatch != nil {
				return nil, fmt.Errorf("multiple pods match %q in namespace %s", name, c.namespace)
			}
			prefixMatch = p
		}
	}
	if prefixMatch != nil {
		return prefixMatch, nil
	}
	return nil, fmt.Errorf("pod %q not found in namespace %s (label %s)", name, c.namespace, AccelerboatAppLabel)
}

func freeLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, err
	}
	return port, nil
}

// ListNodeNames returns the names of all nodes in the cluster.
func (c *Client) ListNodeNames(ctx context.Context) ([]string, error) {
	list, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		names = append(names, list.Items[i].Name)
	}
	return names, nil
}

// ListWorkerNodeNames returns the names of worker nodes only (master/control-plane nodes excluded).
func (c *Client) ListWorkerNodeNames(ctx context.Context) ([]string, error) {
	list, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		n := &list.Items[i]
		if _, hasControlPlane := n.Labels["node-role.kubernetes.io/control-plane"]; hasControlPlane {
			continue
		}
		if _, hasMaster := n.Labels["node-role.kubernetes.io/master"]; hasMaster {
			continue
		}
		names = append(names, n.Name)
	}
	return names, nil
}

// CreateJob creates a Job in the configured namespace.
func (c *Client) CreateJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error) {
	return c.clientset.BatchV1().Jobs(c.namespace).Create(ctx, job, metav1.CreateOptions{})
}

// ListJobs returns Jobs in the configured namespace matching the label selector.
func (c *Client) ListJobs(ctx context.Context, labelSelector string) (*batchv1.JobList, error) {
	return c.clientset.BatchV1().Jobs(c.namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
}

// GetJob returns the named Job in the configured namespace.
func (c *Client) GetJob(ctx context.Context, name string) (*batchv1.Job, error) {
	return c.clientset.BatchV1().Jobs(c.namespace).Get(ctx, name, metav1.GetOptions{})
}

// DeleteJob deletes the named Job in the configured namespace and its dependent Pods (cascading delete).
func (c *Client) DeleteJob(ctx context.Context, name string) error {
	propagation := metav1.DeletePropagationBackground
	return c.clientset.BatchV1().Jobs(c.namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}

// ListPodsBySelector returns pods in the configured namespace matching the label selector.
func (c *Client) ListPodsBySelector(ctx context.Context, labelSelector string) (*corev1.PodList, error) {
	return c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
}

// DeletePod deletes the named Pod in the configured namespace.
func (c *Client) DeletePod(ctx context.Context, name string) error {
	return c.clientset.CoreV1().Pods(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ListEvents returns events in the configured namespace for the given field selector (e.g. involvedObject.name=pod-name).
func (c *Client) ListEvents(ctx context.Context, fieldSelector string) (*corev1.EventList, error) {
	return c.clientset.CoreV1().Events(c.namespace).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector})
}
