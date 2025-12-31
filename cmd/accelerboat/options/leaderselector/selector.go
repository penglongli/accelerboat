// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package leaderselector

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/bk-bcs/bcs-common/common/blog"
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
	MasterIP    string             `json:"masterIP" value:"" usage:"manually specify the master node"`
	PreferNodes *PreferNodesConfig `json:"preferNodes" value:"" usage:"assume the master role and download tasks"`
}

// PreferNodesConfig defines the prefer nodes config
type PreferNodesConfig struct {
	LabelSelectors string `json:"labelSelectors" usage:"the label selector to filter nodes"`
}

var (
	namespace   string
	serviceName string
	preferCfg   *PreferConfig
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

func WatchK8sService(ns, name string, preferConfig *PreferConfig, k8sClientset *kubernetes.Clientset) error {
	namespace = ns
	serviceName = name
	preferCfg = preferConfig
	k8sClient = k8sClientset
	result, err := getServiceEndpoints()
	if err != nil {
		return err
	}
	endpoints = result
	prevMaster := CurrentMaster()
	logger.Infof("current master: %s", prevMaster)

	ctx := context.Background()
	watcher, err := k8swatch.NewRetryWatcherWithContext(ctx, "endpoints", &cache.ListWatch{
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			return k8sClient.CoreV1().Endpoints(ns).Watch(ctx, metav1.ListOptions{
				ResourceVersion: "0",
				FieldSelector:   fmt.Sprintf("metadata.name=%s", name),
			})
		},
	})
	if err != nil {
		return errors.Wrapf(err, "create k8s watcher for endpoints failed")
	}
	go func() {
		defer watcher.Stop()
		logger.Infof("watching k8s endpoint '%s/%s'", ns, name)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					logger.Errorf("watch k8s endpoints channel interrupted")
					return
				}
				if event.Object == nil {
					logger.Errorf("watch k8s endpoints event.object is nil")
					continue
				}
				switch event.Type {
				case watch.Added, watch.Modified, watch.Deleted:
					prevMaster = changeMaster(prevMaster)
				case watch.Error:
					fmt.Printf("watch k8s endpoints error occurred: %v\n", event.Object)
					return
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
	// 获取到用户设置的 Prefer 节点信息列表
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

	// 获取到所有 Image-Proxy 的 Endpoint 节点列表
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
		// 如果存在交集，应该把交集返回回去
		for k := range intersection {
			result = append(result, k)
		}
	} else {
		// 如果无交集，应该把 Endpoints 返回回去
		for k := range epMap {
			result = append(result, k)
		}
	}
	if masterIP := preferCfg.MasterIP; masterIP != "" {
		if _, ok := epMap[masterIP]; !ok {
			logger.Warnf("[master-election] preferConfig.masterIP is specified '%s', but not found", masterIP)
		} else {
			had := false
			// 对比结果是否有 MasterIP，如果没有需要加进去
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
		newResult = append(newResult, fmt.Sprintf("%s:%d", ip, op.HTTPPort))
	}
	logger.Infof("[master-election] get service endpoints: %d", len(newResult))
	return newResult, nil
}
