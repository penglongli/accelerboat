// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

// Package logctx provides a way to log with context
package logctx

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/klog/v2"
)

// ContextKey defines the key of context
type ContextKey string

const (
	// RequestKey 用于记录日志的 Request ID
	RequestKey ContextKey = "t-request"
	// ProxyKey 源仓库地址
	ProxyKey ContextKey = "t-proxy"
	// LayerDigest Layer 的 digest
	LayerDigest ContextKey = "t-layer"
)

var (
	keys = []ContextKey{RequestKey, ProxyKey, LayerDigest}
)

// RequestID get the request-id from context
func RequestID(ctx context.Context) string {
	v := ctx.Value(RequestKey)
	if v == nil {
		return ""
	}
	return v.(string)
}

// GetLayerDigest get the digest from context
func GetLayerDigest(ctx context.Context) string {
	v := ctx.Value(LayerDigest)
	if v == nil {
		return ""
	}
	return v.(string)
}

// SetRequestID set the request-id
func SetRequestID(ctx context.Context, requestID string) context.Context {
	v := ctx.Value(RequestKey)
	if v != nil && v != "" {
		return ctx
	}
	return context.WithValue(ctx, RequestKey, requestID)
}

// SetProxyHost set the original registry
func SetProxyHost(ctx context.Context, proxyHost string) context.Context {
	v := ctx.Value(ProxyKey)
	if v != nil && v != "" {
		return ctx
	}
	return context.WithValue(ctx, ProxyKey, proxyHost)
}

// SetLayerDigest set layer digest
func SetLayerDigest(ctx context.Context, layerDigest string) context.Context {
	v := ctx.Value(LayerDigest)
	if v != nil && v != "" {
		return ctx
	}
	return context.WithValue(ctx, LayerDigest, layerDigest)
}

func getKeyValueFromCtx(ctx context.Context) string {
	result := make([]string, 0)
	for i := range keys {
		v := ctx.Value(keys[i])
		if v != nil {
			result = append(result, string(keys[i])+"="+v.(string))
		}
	}
	if len(result) == 0 {
		return ""
	}
	return ", " + strings.Join(result, ", ")
}

// Infof Info 级别日志
func Infof(ctx context.Context, format string, args ...interface{}) {
	klog.InfoDepth(1, fmt.Sprintf(format+getKeyValueFromCtx(ctx), args...))
}

// Warnf Warn 级别日志
func Warnf(ctx context.Context, format string, args ...interface{}) {
	klog.WarningDepth(1, fmt.Sprintf(format+getKeyValueFromCtx(ctx), args...))
}

// Errorf Error 级别日志
func Errorf(ctx context.Context, format string, args ...interface{}) {
	klog.ErrorDepth(1, fmt.Sprintf(format+getKeyValueFromCtx(ctx), args...))
}

// Fatalf Fatal 级别日志
func Fatalf(ctx context.Context, format string, args ...interface{}) {
	klog.FatalDepth(1, fmt.Sprintf(format+getKeyValueFromCtx(ctx), args...))
}
