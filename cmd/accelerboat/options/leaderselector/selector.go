// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package leaderselector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	k8swatch "k8s.io/client-go/tools/watch"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/utils"
)

// PreferConfig defines the prefer config
type PreferConfig struct {
	MasterIP    string            `json:"masterIP"`
	PreferNodes PreferNodesConfig `json:"preferNodes"`
}

// PreferNodesConfig defines the prefer nodes config
type PreferNodesConfig struct {
	LabelSelectors string `json:"labelSelectors" usage:"the label selector to filter nodes"`
}

var (
	namespace   string
	serviceName string
	serverPort  int64
	preferCfg   PreferConfig
	k8sClient   *kubernetes.Clientset

	endpoints []string
)

func changeMaster(prevMaster string) string {
	result, err := getServiceEndpoints()
	if err != nil {
		logger.Errorf("get service endpoints failed: %s", err.Error())
	} else {
		endpoints = result
		currentMaster := CurrentMaster()
		if prevMaster != currentMaster {
			logger.Infof("current master: %s => %s", prevMaster, currentMaster)
			return currentMaster
		}
	}
	return prevMaster
}

// Endpoints returns the service endpoints
func Endpoints() []string {
	return endpoints
}

// CurrentMaster return the current master
func CurrentMaster() string {
	var currentASCII int64 = 0
	var currentEndpoint string
	masterIP := preferCfg.MasterIP
	for i := range endpoints {
		ep := endpoints[i]
		if masterIP != "" && strings.HasPrefix(ep, masterIP+":") {
			return ep
		}
		ascii := utils.StringASCII(ep)
		if currentASCII < ascii {
			currentASCII = ascii
			currentEndpoint = ep
		}
	}
	return currentEndpoint
}

func createEndpointsWatcher() (*k8swatch.RetryWatcher, error) {
	epList, err := k8sClient.CoreV1().Endpoints(namespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", serviceName),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "list k8s endpoints failed")
	}
	watcher, err := k8swatch.NewRetryWatcherWithContext(context.Background(), epList.ResourceVersion,
		&cache.ListWatch{
			WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
				return k8sClient.CoreV1().Endpoints(namespace).Watch(ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("metadata.name=%s", serviceName),
				})
			},
		})
	if err != nil {
		return nil, errors.Wrapf(err, "create k8s watcher for endpoints failed")
	}
	return watcher, nil
}

func WatchK8sService(ns, name string, port int64, preferConfig PreferConfig,
	k8sClientSet *kubernetes.Clientset) error {
	{
		namespace = ns
		serviceName = name
		serverPort = port
		preferCfg = preferConfig
		k8sClient = k8sClientSet
	}
	result, err := getServiceEndpoints()
	if err != nil {
		return err
	}
	endpoints = result
	prevMaster := CurrentMaster()
	logger.Infof("current master: %s", prevMaster)

	watcher, err := createEndpointsWatcher()
	if err != nil {
		return err
	}
	go func() {
		defer func() {
			watcher.Stop()
			logger.Infof("k8s endpoints watcher stopped")
		}()
		logger.Infof("watching k8s endpoint '%s/%s'", ns, name)
		for {
			select {
			case event, ok := <-watcher.ResultChan():
				if !ok {
					logger.Errorf("watch k8s endpoints channel interrupted")
					break
				}
				if event.Object == nil {
					logger.Errorf("watch k8s endpoints event.object is nil")
					break
				}
				switch event.Type {
				case watch.Added, watch.Modified, watch.Deleted:
					prevMaster = changeMaster(prevMaster)
				case watch.Error:
					fmt.Printf("watch k8s endpoints error occurred: %v\n", event.Object)
					break
				}
			case <-watcher.Done():
				logger.Errorf("k8s endpoints watcher closed with unexpected error")
				var newWatcher *k8swatch.RetryWatcher
				newWatcher, err = createEndpointsWatcher()
				if err != nil {
					logger.Errorf("create endpoints watcher failed")
					time.Sleep(time.Second)
				} else {
					logger.Infof("create endpoints watcher success")
					watcher = newWatcher
				}
			}
		}
	}()
	return nil
}

func mapKeys(m map[string]struct{}) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

func getServiceEndpoints() ([]string, error) {
	var preferNodes = make(map[string]struct{})
	// Get user-configured preferred nodes
	if selectors := preferCfg.PreferNodes.LabelSelectors; selectors != "" {
		nodeList, err := k8sClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{
			LabelSelector: selectors,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "list nodes by selector '%s' failed", selectors)
		}
		for i := range nodeList.Items {
			addresses := nodeList.Items[i].Status.Addresses
			for _, address := range addresses {
				if address.Type != corev1.NodeInternalIP {
					continue
				}
				preferNodes[address.Address] = struct{}{}
			}
		}
		if len(preferNodes) == 0 {
			logger.Warnf("[master-election] there not get any nodes by prefer-selectors: %s", selectors)
		}
	}

	eps, err := k8sClient.CoreV1().Endpoints(namespace).Get(context.Background(), serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "get k8s endpoint '%s/%s' failed", namespace, serviceName)
	}
	epMap := make(map[string]struct{})
	intersection := map[string]struct{}{}
	for i := range eps.Subsets {
		ep := &eps.Subsets[i]
		for j := range ep.Addresses {
			epIP := ep.Addresses[j].IP
			epMap[epIP] = struct{}{}
			if _, ok := preferNodes[epIP]; ok {
				intersection[epIP] = struct{}{}
			}
		}
	}
	result := make([]string, 0)
	if len(intersection) != 0 {
		logger.Infof("[master-election] get prefer-nodes with endpoints intersection: %v",
			mapKeys(intersection))
		for k := range intersection {
			result = append(result, k)
		}
	} else {
		for k := range epMap {
			result = append(result, k)
		}
	}
	if masterIP := preferCfg.MasterIP; masterIP != "" {
		if _, ok := epMap[masterIP]; !ok {
			logger.Warnf("[master-election] preferConfig.masterIP is specified '%s', but not found", masterIP)
		} else {
			had := false
			for _, ip := range result {
				if ip == masterIP {
					had = true
					break
				}
			}
			if !had {
				result = append(result, masterIP)
			}
		}
	}
	newResult := make([]string, 0, len(result))
	for _, ip := range result {
		newResult = append(newResult, fmt.Sprintf("%s:%d", ip, serverPort))
	}
	logger.Infof("[master-election] get service endpoints: %d", len(newResult))
	return newResult, nil
}
