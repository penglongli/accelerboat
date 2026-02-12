// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olekukonko/tablewriter"

	"github.com/penglongli/accelerboat/pkg/recorder"
	"github.com/penglongli/accelerboat/pkg/utils/formatutils"
)

const (
	recorderLimitDefault    = 300
	recorderRepoOrExtraWrap = 50
	recorderMessageWrap     = 120 // Message column wrap width (chars)
)

func recorderLimitFromQuery(c *gin.Context) int {
	limit := recorderLimitDefault
	if s := c.Query("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	return limit
}

// eventToMap returns a map suitable for JSON response (type, timestamp, requestID, eventStatus, details, message).
func eventToMap(e recorder.Event) map[string]interface{} {
	return map[string]interface{}{
		"type":        string(e.Type),
		"timestamp":   e.Timestamp,
		"requestID":   e.RequestID,
		"eventStatus": string(e.EventStatus),
		"details":     e.Details,
		"message":     e.Message,
	}
}

// formatEventType converts snake_case to PascalCase for display, e.g. serve_blob_from_local -> ServeBlobFromLocal.
func formatEventType(typeStr string) string {
	if typeStr == "" {
		return typeStr
	}
	parts := strings.Split(typeStr, "_")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	return strings.Join(parts, "")
}

// detailStr extracts a string from details for display; empty if missing or not string.
func detailStr(d map[string]interface{}, key string) string {
	if d == nil {
		return ""
	}
	v, ok := d[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// detailDurationSec extracts duration_ms from details and returns duration in seconds as string (e.g. "0.342"). Returns "-" if missing.
func detailDurationSec(d map[string]interface{}) string {
	if d == nil {
		return "-"
	}
	v, ok := d["duration_ms"]
	if !ok || v == nil {
		return "-"
	}
	var ms float64
	switch x := v.(type) {
	case float64:
		ms = x
	case int:
		ms = float64(x)
	case int64:
		ms = float64(x)
	default:
		return "-"
	}
	if ms < 0 {
		return "-"
	}
	sec := ms / 1000
	return fmt.Sprintf("%.3f", sec)
}

// formatRelativeTime formats t as a k8s-style "ago" string: 5s, 1m, 2h3m, 2d, 1y2d, etc.
func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		return "0s"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if d < 365*24*time.Hour {
		days := int(d / (24 * time.Hour))
		h := int(d.Hours()) % 24
		if h == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd%dh", days, h)
	}
	years := int(d / (365 * 24 * time.Hour))
	days := int(d/(24*time.Hour)) % 365
	if days == 0 {
		return fmt.Sprintf("%dy", years)
	}
	return fmt.Sprintf("%dy%dd", years, days)
}

// wrapMessage breaks message into lines of at most width runes, at word boundaries when possible.
func wrapMessage(msg string, width int) string {
	if width <= 0 || len(msg) <= width {
		return msg
	}
	var b strings.Builder
	runes := []rune(msg)
	start := 0
	for start < len(runes) {
		end := start + width
		if end > len(runes) {
			end = len(runes)
		} else {
			// try to break at last space in this chunk
			lastSpace := -1
			for i := start; i < end; i++ {
				if runes[i] == ' ' || runes[i] == '\n' {
					lastSpace = i
				}
			}
			if lastSpace >= start {
				end = lastSpace + 1
			}
		}
		if b.Len() > 0 {
			b.WriteRune('\n')
		}
		b.WriteString(string(runes[start:end]))
		start = end
		for start < len(runes) && (runes[start] == ' ' || runes[start] == '\n') {
			start++
		}
	}
	return b.String()
}

func convertString(v interface{}) string {
	if v == nil {
		return ""
	}
	return v.(string)
}

func convertInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch v.(type) {
	case int64:
		return v.(int64)
	case float64:
		return int64(v.(float64))
	}
	return 0
}

func buildExtra(e *recorder.Event) string {
	details := make([]string, 0)
	switch e.Type {
	case recorder.EventTypeReverseProxy:
		details = append(details, "method="+convertString(e.Details["method"]))
		details = append(details, "path="+convertString(e.Details["path"]))
	case recorder.EventTypeServiceToken:
		details = append(details, "master="+convertString(e.Details["master"]))
		details = append(details, "scope="+convertString(e.Details["scope"]))
	case recorder.EventTypeHeadManifest:
		details = append(details, "master="+convertString(e.Details["master"]))
		details = append(details, "tag="+convertString(e.Details["tag"]))
	case recorder.EventTypeGetManifest:
		details = append(details, "master="+convertString(e.Details["master"]))
		details = append(details, "tag="+convertString(e.Details["tag"]))
	case recorder.EventServeBlobFromLocal:
		details = append(details, "digest="+convertString(e.Details["digest"]))
		details = append(details, "size="+formatutils.FormatSize(convertInt64(e.Details["size"])))
	case recorder.EventTypeGetBlobFromMaster:
		details = append(details, "digest="+convertString(e.Details["digest"]))
		if target := e.Details["target"]; target != nil {
			details = append(details, "target="+convertString(target))
		}
		if file := e.Details["file"]; file != nil {
			details = append(details, "file="+convertString(file))
		}
		if size := e.Details["size"]; size != nil {
			details = append(details, "size="+formatutils.FormatSize(convertInt64(size)))
		}
	case recorder.EventTypeDownloadBlobByTCP, recorder.EventTypeDownloadBlobByTorrent:
		details = append(details, "digest="+convertString(e.Details["digest"]))
		details = append(details, "target="+convertString(e.Details["target"]))
		details = append(details, "file="+convertString(e.Details["file"]))
		details = append(details, "size="+formatutils.FormatSize(convertInt64(e.Details["size"])))
	}
	return strings.Join(details, "\n")
}

// filterRecorderEvents filters events by query params: registry (exact match) and search (substring match on repoOrExtra).
func filterRecorderEvents(events []recorder.Event, registry, search string) []recorder.Event {
	if registry == "" && search == "" {
		return events
	}
	out := make([]recorder.Event, 0, len(events))
	for _, e := range events {
		if registry != "" && detailStr(e.Details, "registry") != registry {
			continue
		}
		if search != "" && !strings.Contains(buildExtra(&e), search) &&
			!strings.Contains(detailStr(e.Details, "repo"), search) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// eventMatchesFilter returns true if the event passes registry (exact) and search (substring on repoOrExtra).
func eventMatchesFilter(e *recorder.Event, registry, search string) bool {
	if registry != "" && detailStr(e.Details, "registry") != registry {
		return false
	}
	if search != "" && !strings.Contains(buildExtra(e), search) &&
		!strings.Contains(detailStr(e.Details, "repo"), search) {
		return false
	}
	return true
}

func formatRecorderEventsTable(events []recorder.Event) string {
	var b strings.Builder
	tbl := tablewriter.NewWriter(&b)
	tbl.SetHeader([]string{"Timestamp", "Type", "Status", "Registry", "Repo", "Duration", "Message", "Extra"})
	tbl.SetAlignment(tablewriter.ALIGN_LEFT)
	tbl.SetBorder(true)
	tbl.SetColWidth(recorderMessageWrap)
	for _, e := range events {
		registry := detailStr(e.Details, "registry")
		repo := detailStr(e.Details, "repo")
		if repo == "" {
			repo = "-"
		} else {
			repo = wrapMessage(repo, recorderRepoOrExtraWrap)
		}
		msg := e.Message
		if msg == "" {
			msg = "-"
		} else {
			msg = wrapMessage(msg, recorderMessageWrap)
		}
		extra := buildExtra(&e)
		dur := detailDurationSec(e.Details)
		ts := formatRelativeTime(e.Timestamp)
		typeStr := formatEventType(string(e.Type))
		tbl.Append([]string{ts, typeStr, string(e.EventStatus), registry, repo, dur, msg, extra})
	}
	tbl.Render()
	return b.String()
}

// RecorderOutput returns (jsonData, tableText, error) for the recorder API (no follow).
// Query params: limit, registry (exact match), search (substring match on repoOrExtra).
func (h *CustomHandler) RecorderOutput(c *gin.Context) (interface{}, string, error) {
	limit := recorderLimitFromQuery(c)
	registry := strings.TrimSpace(c.Query("registry"))
	search := strings.TrimSpace(c.Query("search"))
	events := recorder.Global.List(limit, []string{search}, nil)
	if events == nil {
		events = []recorder.Event{}
	}
	events = filterRecorderEvents(events, registry, search)
	out := make([]interface{}, 0, len(events))
	for _, e := range events {
		out = append(out, eventToMap(e))
	}
	jsonData := gin.H{"events": out}
	text := formatRecorderEventsTable(events)
	return jsonData, text, nil
}

// recorderStream handles follow=true: stream initial events then new events until client disconnects.
// Query params: limit, registry (exact match), search (substring match on repoOrExtra).
func (h *CustomHandler) recorderStream(c *gin.Context) {
	limit := recorderLimitFromQuery(c)
	outputJSON := c.Query("output") == "json"
	registryFilter := strings.TrimSpace(c.Query("registry"))
	searchFilter := strings.TrimSpace(c.Query("search"))

	events := recorder.Global.List(limit, []string{searchFilter}, nil)
	if events == nil {
		events = []recorder.Event{}
	}
	events = filterRecorderEvents(events, registryFilter, searchFilter)

	w := c.Writer
	header := w.Header()
	header.Set("Transfer-Encoding", "chunked")
	header.Set("X-Content-Type-Options", "nosniff")
	if outputJSON {
		header.Set("Content-Type", "application/json; charset=utf-8")
	} else {
		header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Write initial batch
	if outputJSON {
		enc := json.NewEncoder(w)
		for _, e := range events {
			_ = enc.Encode(eventToMap(e))
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	} else {
		_, _ = w.Write([]byte(formatRecorderEventsTable(events)))
		_, _ = w.Write([]byte("\n--- (follow) ---\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	ch, unsub := recorder.Global.Subscribe()
	defer unsub()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if !eventMatchesFilter(&e, registryFilter, searchFilter) {
				continue
			}
			if outputJSON {
				_ = json.NewEncoder(w).Encode(eventToMap(e))
			} else {
				registry := detailStr(e.Details, "registry")
				repo := detailStr(e.Details, "repo")
				if repo == "" {
					repo = "-"
				} else {
					repo = wrapMessage(repo, recorderRepoOrExtraWrap)
				}
				msg := e.Message
				if msg == "" {
					msg = "-"
				} else {
					msg = wrapMessage(msg, recorderMessageWrap)
				}
				extra := buildExtra(&e)
				dur := detailDurationSec(e.Details)
				ts := formatRelativeTime(e.Timestamp)
				typeStr := formatEventType(string(e.Type))
				row := strings.Join([]string{ts, typeStr, string(e.EventStatus), registry, repo, dur, msg, extra}, "\t")
				_, _ = w.Write([]byte(row + "\n"))
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}

// RecorderHandler handles GET /customapi/recorder with optional query: output=json, limit=N, follow=true, registry=<exact>, search=<substring>.
func (h *CustomHandler) RecorderHandler(c *gin.Context) {
	if c.Query("follow") == "true" {
		h.recorderStream(c)
		return
	}
	jsonData, text, err := h.RecorderOutput(c)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if c.Query("output") == "json" {
		c.JSON(http.StatusOK, jsonData)
		return
	}
	c.String(http.StatusOK, text)
}

// TorrentStatus returns the current torrent status (placeholder implementation).
func (h *CustomHandler) TorrentStatus(c *gin.Context) (interface{}, error) {
	cl := h.torrentHandler.GetClient()
	cl.WriteStatus(c.Writer)
	return nil, nil
}
