// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// MetricFamily 表示一个指标族，items 为强类型列表。
type MetricFamily struct {
	Items []MetricItem `json:"items"`
}

// MetricItem 单条指标：Counter/Gauge 用 Value，Histogram 用 Count+Sum。
type MetricItem struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value,omitempty"`
	Count  uint64            `json:"count,omitempty"`
	Sum    float64           `json:"sum,omitempty"`
}

// customMetricNames 仅包含 pkg/metrics 中定义的自定义指标名（Prometheus 采集后的名称）。
var customMetricNames = []string{
	"accelerboat_http_requests_total",
	"accelerboat_http_request_duration_seconds",
	"accelerboat_registry_requests_total",
	"accelerboat_registry_request_duration_seconds",
	"accelerboat_redis_operations_total",
	"redis_operation_duration",
	"accelerboat_torrent_operations_total",
	"accelerboat_torrent_operation_duration_seconds",
	"accelerboat_torrent_active_count",
	"accelerboat_transfer_size",
	"accelerboat_disk_usage",
	"accelerboat_errors_total",
}

func (h *CustomHandler) Metrics(c *gin.Context) (interface{}, string, error) {
	data, err := gatherMetricsReadable()
	if err != nil {
		return nil, "", err
	}
	text := formatMetricsReadable(data)
	return data, text, nil
}

func gatherMetricsReadable() (map[string]MetricFamily, error) {
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}
	nameToFamily := make(map[string]*dto.MetricFamily)
	for _, mf := range metricFamilies {
		if mf.Name != nil {
			nameToFamily[*mf.Name] = mf
		}
	}
	out := make(map[string]MetricFamily, len(customMetricNames))
	for _, name := range customMetricNames {
		mf := nameToFamily[name]
		if mf == nil {
			continue
		}
		out[name] = buildMetricFamily(mf)
	}
	return out, nil
}

func buildMetricFamily(mf *dto.MetricFamily) MetricFamily {
	var items []MetricItem
	for _, m := range mf.Metric {
		labels := make(map[string]string)
		for _, l := range m.Label {
			if l.Name != nil && l.Value != nil {
				labels[*l.Name] = *l.Value
			}
		}
		if m.Histogram != nil {
			items = append(items, MetricItem{
				Labels: labels,
				Count:  m.Histogram.GetSampleCount(),
				Sum:    m.Histogram.GetSampleSum(),
			})
		} else {
			var value float64
			if m.Counter != nil && m.Counter.Value != nil {
				value = *m.Counter.Value
			} else if m.Gauge != nil && m.Gauge.Value != nil {
				value = *m.Gauge.Value
			} else if m.Untyped != nil && m.Untyped.Value != nil {
				value = *m.Untyped.Value
			} else {
				continue
			}
			items = append(items, MetricItem{Labels: labels, Value: value})
		}
	}
	return MetricFamily{Items: items}
}

// StatsMetrics 供 /customapi/stats 使用的聚合指标（磁盘、流量、错误、种子数）。
type StatsMetrics struct {
	DiskUsage         map[string]float64 // label -> GB
	TransferSize      map[string]float64 // operation -> GB
	ErrorsTotal       int64
	TorrentActiveCount int
}

// getStatsMetrics 从 Prometheus 采集并汇总 stats 所需的四项指标。
func getStatsMetrics() (StatsMetrics, error) {
	out := StatsMetrics{
		DiskUsage:    make(map[string]float64),
		TransferSize: make(map[string]float64),
	}
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return out, err
	}
	nameToFamily := make(map[string]*dto.MetricFamily)
	for _, mf := range families {
		if mf.Name != nil {
			nameToFamily[*mf.Name] = mf
		}
	}
	if mf := nameToFamily["accelerboat_disk_usage"]; mf != nil {
		for _, m := range mf.Metric {
			if m.Gauge == nil || m.Gauge.Value == nil {
				continue
			}
			var path string
			for _, l := range m.Label {
				if l.Name != nil && l.Value != nil && *l.Name == "path" {
					path = *l.Value
					break
				}
			}
			out.DiskUsage[path] = *m.Gauge.Value
		}
	}
	if mf := nameToFamily["accelerboat_transfer_size"]; mf != nil {
		for _, m := range mf.Metric {
			if m.Counter == nil || m.Counter.Value == nil {
				continue
			}
			var op string
			for _, l := range m.Label {
				if l.Name != nil && l.Value != nil && *l.Name == "operation" {
					op = *l.Value
					break
				}
			}
			out.TransferSize[op] = *m.Counter.Value
		}
	}
	if mf := nameToFamily["accelerboat_errors_total"]; mf != nil {
		for _, m := range mf.Metric {
			if m.Counter != nil && m.Counter.Value != nil {
				out.ErrorsTotal += int64(*m.Counter.Value)
			}
		}
	}
	if mf := nameToFamily["accelerboat_torrent_active_count"]; mf != nil {
		for _, m := range mf.Metric {
			if m.Gauge != nil && m.Gauge.Value != nil {
				out.TorrentActiveCount = int(*m.Gauge.Value)
				break
			}
		}
	}
	return out, nil
}

func formatMetricsReadable(data map[string]MetricFamily) string {
	var b strings.Builder
	b.WriteString("=== AccelerBoat Metrics ===\n\n")
	for _, name := range customMetricNames {
		family, ok := data[name]
		if !ok || len(family.Items) == 0 {
			continue
		}
		appendMetricSection(&b, name, family.Items)
	}
	return b.String()
}

func appendMetricSection(b *strings.Builder, name string, items []MetricItem) {
	title := metricDisplayTitle(name)
	b.WriteString(title)
	b.WriteString("\n")

	switch name {
	case "accelerboat_http_requests_total":
		writeTable(b, items, []string{"method", "path", "status"}, "count", true)
	case "accelerboat_http_request_duration_seconds":
		writeHistogramTable(b, items, []string{"method", "path"})
		writeHistogramSummary(b, items)
	case "accelerboat_registry_requests_total":
		writeTable(b, items, []string{"registry", "type", "status"}, "count", true)
	case "accelerboat_registry_request_duration_seconds":
		writeHistogramTable(b, items, []string{"host", "type"})
		writeHistogramSummary(b, items)
	case "accelerboat_redis_operations_total":
		writeTable(b, items, []string{"operation", "status"}, "count", true)
	case "redis_operation_duration":
		writeHistogramTable(b, items, []string{"operation", "status"})
	case "accelerboat_torrent_operations_total":
		writeTable(b, items, []string{"operation", "status"}, "count", true)
	case "accelerboat_torrent_operation_duration_seconds":
		writeHistogramTable(b, items, []string{"operation"})
		writeHistogramSummary(b, items)
	case "accelerboat_torrent_active_count":
		writeSingleValue(b, items)
	case "accelerboat_transfer_size":
		writeTable(b, items, []string{"operation"}, "GB", false)
	case "accelerboat_disk_usage":
		writeTable(b, items, []string{"path"}, "GB", false)
	case "accelerboat_errors_total":
		writeTable(b, items, []string{"component", "action"}, "count", true)
	default:
		writeGenericRows(b, items)
	}
	b.WriteString("\n")
}

func metricDisplayTitle(name string) string {
	titles := map[string]string{
		"accelerboat_http_requests_total":                "HTTP 请求 (按 method / path / status)",
		"accelerboat_http_request_duration_seconds":      "HTTP 请求耗时",
		"accelerboat_registry_requests_total":            "Registry 代理请求 (按 registry / type / status)",
		"accelerboat_registry_request_duration_seconds":  "Registry 请求耗时",
		"accelerboat_redis_operations_total":             "Redis 操作 (按 operation / status)",
		"redis_operation_duration":                       "Redis 操作耗时",
		"accelerboat_torrent_operations_total":           "Torrent 操作 (按 operation / status)",
		"accelerboat_torrent_operation_duration_seconds": "Torrent 操作耗时",
		"accelerboat_torrent_active_count":               "当前活跃 Torrent 数量",
		"accelerboat_transfer_size":                      "传输体积 (GB)",
		"accelerboat_disk_usage":                         "磁盘占用 (GB)",
		"accelerboat_errors_total":                       "错误统计 (按 component / action)",
	}
	if t, ok := titles[name]; ok {
		return t
	}
	return name
}

func writeHistogramTable(b *strings.Builder, items []MetricItem, labelKeys []string) {
	if len(items) == 0 {
		return
	}
	header := append([]string{}, labelKeys...)
	header = append(header, "Count", "Sum", "Avg")
	tbl := tablewriter.NewWriter(b)
	tbl.SetHeader(header)
	tbl.SetAlignment(tablewriter.ALIGN_LEFT)
	tbl.SetBorder(true)
	for _, it := range items {
		row := make([]string, 0, len(header))
		for _, k := range labelKeys {
			row = append(row, it.Labels[k])
		}
		avg := ""
		if it.Count > 0 {
			avg = fmt.Sprintf("%.4g", it.Sum/float64(it.Count))
		}
		row = append(row, strconv.FormatUint(it.Count, 10), fmt.Sprintf("%.4g", it.Sum), avg)
		tbl.Append(row)
	}
	tbl.Render()
}

func writeTable(b *strings.Builder, items []MetricItem, labelKeys []string, valueLabel string, asInt bool) {
	if len(items) == 0 {
		return
	}
	valueCol := "Value"
	if valueLabel != "" {
		valueCol = valueLabel
	}
	header := append([]string{}, labelKeys...)
	header = append(header, valueCol)
	tbl := tablewriter.NewWriter(b)
	tbl.SetHeader(header)
	tbl.SetAlignment(tablewriter.ALIGN_LEFT)
	tbl.SetBorder(true)
	for _, it := range items {
		row := make([]string, 0, len(header))
		for _, k := range labelKeys {
			row = append(row, it.Labels[k])
		}
		cell := fmt.Sprintf("%.4g", it.Value)
		if asInt {
			cell = fmt.Sprintf("%.0f", it.Value)
		}
		row = append(row, cell)
		tbl.Append(row)
	}
	tbl.Render()
}

func writeHistogramSummary(b *strings.Builder, items []MetricItem) {
	var totalCount uint64
	var totalSum float64
	for _, it := range items {
		totalCount += it.Count
		totalSum += it.Sum
	}
	if totalCount == 0 {
		return
	}
	avg := totalSum / float64(totalCount)
	b.WriteString(fmt.Sprintf("  [汇总] 请求数: %d, 总耗时: %.4g s, 平均: %.4g s\n", totalCount, totalSum, avg))
}

func writeSingleValue(b *strings.Builder, items []MetricItem) {
	if len(items) == 0 {
		return
	}
	tbl := tablewriter.NewWriter(b)
	tbl.SetHeader([]string{"Value"})
	tbl.SetBorder(true)
	tbl.Append([]string{fmt.Sprintf("%d", int64(items[0].Value))})
	tbl.Render()
}

func writeGenericRows(b *strings.Builder, items []MetricItem) {
	if len(items) == 0 {
		return
	}
	// 收集所有 label keys 并排序
	keySet := make(map[string]struct{})
	for _, it := range items {
		for k := range it.Labels {
			keySet[k] = struct{}{}
		}
	}
	var labelKeys []string
	for k := range keySet {
		labelKeys = append(labelKeys, k)
	}
	sort.Strings(labelKeys)
	header := append([]string{}, labelKeys...)
	header = append(header, "Value")
	tbl := tablewriter.NewWriter(b)
	tbl.SetHeader(header)
	tbl.SetAlignment(tablewriter.ALIGN_LEFT)
	tbl.SetBorder(true)
	for _, it := range items {
		row := make([]string, 0, len(header))
		for _, k := range labelKeys {
			row = append(row, it.Labels[k])
		}
		row = append(row, fmt.Sprintf("%.4g", it.Value))
		if it.Count > 0 {
			row[len(row)-1] = fmt.Sprintf("%.4g (count=%d sum=%v)", it.Value, it.Count, it.Sum)
		}
		tbl.Append(row)
	}
	tbl.Render()
}
