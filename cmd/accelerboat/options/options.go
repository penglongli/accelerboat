// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
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
		Timeout:   5 * time.Second,
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

// FilterRegistryMapping filter registry mapping
func (o *AccelerBoatOption) FilterRegistryMapping(proxyHost string, proxyType ProxyType) *RegistryMapping {
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
	if proxyType == RegistryMirror {
		return &RegistryMapping{
			Enable:       true,
			ProxyHost:    proxyHost,
			OriginalHost: proxyHost,
		}
	}
	return nil
}

// FilterRegistryMappingByOriginal filter registry mappings by original registry
func (o *AccelerBoatOption) FilterRegistryMappingByOriginal(originalHost string) *RegistryMapping {
	for _, m := range o.ExternalConfig.RegistryMappings {
		if originalHost == m.OriginalHost {
			return m
		}
	}
	return nil
}
