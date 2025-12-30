// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/penglongli/accelerboat/pkg/utils"
)

// ProxyType defines proxy type
type ProxyType string

const (
	// DomainProxy domain proxy
	DomainProxy ProxyType = "DomainProxy"
	// RegistryMirror registry mirror
	RegistryMirror ProxyType = "RegistryMirror"
)

// HTTPProxyTransport return the insecure-skip-verify transport
func (o *AccelerBoatOption) HTTPProxyTransport() http.RoundTripper {
	netDialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	tp := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           netDialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	}
	if o.ExternalConfig.HTTPProxyUrl == nil {
		return tp
	}
	tp.Proxy = http.ProxyURL(o.ExternalConfig.HTTPProxyUrl)
	return tp
}

// CurrentMaster return the current master
func (o *AccelerBoatOption) CurrentMaster() string {
	var currentASCII int64 = 0
	var currentEndpoint string
	masterIP := o.PreferConfig.MasterIP
	for i := range o.ServiceDiscovery.Endpoints {
		ep := o.ServiceDiscovery.Endpoints[i]
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

// FilterRegistryMapping filter registry mapping
func (o *AccelerBoatOption) FilterRegistryMapping(proxyHost string, proxyType ProxyType) *RegistryMapping {
	// 针对 ProxyHost 为空，设置其默认使用 docker.io
	if proxyHost == "" {
		return &o.ExternalConfig.DockerHubRegistry
	}
	for _, m := range o.ExternalConfig.RegistryMappings {
		switch proxyType {
		case RegistryMirror:
			// for containerd
			if proxyHost == m.OriginalHost {
				return m
			}
			// for dockerd
			if proxyHost == m.ProxyHost {
				return m
			}
		case DomainProxy:
			if proxyHost == m.ProxyHost {
				return m
			}
		}
	}
	return nil
}

// FilterRegistryMappingByOriginal filter registry mappings by original registry
func (o *AccelerBoatOption) FilterRegistryMappingByOriginal(originalHost string) *RegistryMapping {
	if o.ExternalConfig.DockerHubRegistry.OriginalHost == originalHost {
		return &o.ExternalConfig.DockerHubRegistry
	}
	for _, m := range o.ExternalConfig.RegistryMappings {
		if originalHost == m.OriginalHost {
			return m
		}
	}
	return nil
}
