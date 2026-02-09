// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

// Package metrics provides Prometheus metrics for accelerboat.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "accelerboat"

// Component constants for ErrorsTotal label.
const (
	ComponentOCIScan      = "ociscan"
	ComponentReverseProxy = "reverse_proxy"
	ComponentRedis        = "redis"
)

// RecordError increments the errors_total counter for the given component, operation and error type.
// operation can be empty for single-operation components.
func RecordError(component, action string) {
	ErrorsTotal.WithLabelValues(component, action).Inc()
}

var (
	// HTTPRequestsTotal HTTP request metrics (all HTTP traffic)
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests by method, path and status.",
		},
		[]string{"target", "method", "path", "status"},
	)

	HTTPRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"target", "method", "path"},
	)

	// RegistryRequestsTotal Registry proxy metrics (v2 registry traffic: auth, manifest, blob)
	RegistryRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "registry_requests_total",
			Help:      "Total number of registry proxy requests by type, source and status.",
		},
		[]string{"registry", "type", "status"},
	)

	RegistryRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "registry_request_duration_seconds",
			Help:      "Registry proxy request latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"host", "type"},
	)

	// RedisOperationsTotal Redis / cache store metrics
	RedisOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "redis_operations_total",
			Help:      "Total number of Redis cache operations by operation and status.",
		},
		[]string{"operation", "status"},
	)
	// RedisOperationDuration Redis operation cost time
	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_operation_duration",
			Help:    "Duration of Redis operations in milliseconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)

	// TorrentOperationsTotal BitTorrent metrics
	TorrentOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "torrent_operations_total",
			Help:      "Total number of BitTorrent operations by operation and status.",
		},
		[]string{"operation", "status"},
	)
	TorrentOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "torrent_operation_duration_seconds",
			Help:      "Registry proxy request latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation"},
	)
	TorrentActiveCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "torrent_active_count",
			Help:      "Number of active torrents (seeding).",
		},
	)

	// TransferSize defines transferred size
	// download_from_registry, download_by_tcp, download_by_torrent, serve_blob_by_tcp, serve_blob_from_local
	TransferSize = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "transfer_size",
			Help:      "Total size transferred for layer(unit: GB)",
		},
		[]string{"operation"},
	)

	// DiskUsage defines the current disk used per storage path (unit: GB).
	DiskUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "disk_usage",
			Help:      "Current disk usage per storage path (unit: GB)",
		},
		[]string{"path"},
	)

	// ErrorsTotal counts errors by component, operation and error_type (for alerting and debugging).
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "errors_total",
			Help:      "Total number of errors by component, operation and error type.",
		},
		[]string{"component", "action"},
	)
)
