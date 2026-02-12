// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

// Package recorder provides event recording for image pull operations (service token,
// manifest, blob, layer cache, etc.) for observability and debugging. Events are stored
// in an in-memory ring buffer and can be queried via the recorder API. Optionally,
// events are also written to a rotating file when InitEventFile is called. File writes
// are asynchronous (non-blocking) to avoid slowing down the hot path.
package recorder

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/common"
	"github.com/penglongli/accelerboat/pkg/utils"
)

const (
	// DefaultBufferSize is the default maximum number of events to retain.
	DefaultBufferSize = 1000
	// DefaultEventFileMaxSizeMB is the default max size in MB before rotating (1GB).
	DefaultEventFileMaxSizeMB = 1024
	// DefaultEventFileMaxBackups is the default number of rotated files to keep.
	DefaultEventFileMaxBackups = 5
	// eventFileChanCap is the buffer size for async file writes. When full, new events are dropped for file only (ring buffer still updated).
	eventFileChanCap = 10000
	// eventFileFlushInterval is how often the file writer flushes to disk when idle.
	eventFileFlushInterval = 100 * time.Millisecond
)

// EventType represents the kind of operation that was recorded.
type EventType string

const (
	EventTypeServiceToken          EventType = "service_token"
	EventTypeHeadManifest          EventType = "head_manifest"
	EventTypeGetManifest           EventType = "get_manifest"
	EventServeBlobFromLocal        EventType = "serve_blob_from_local"
	EventTypeGetBlobFromMaster     EventType = "get_blob_from_master"
	EventTypeDownloadBlobByTCP     EventType = "download_blob_by_tcp"
	EventTypeDownloadBlobByTorrent EventType = "download_blob_by_torrent"
	EventTypeGetLayerInfo          EventType = "get_layer_info"
	EventTypeDownloadLayer         EventType = "download_layer"
	EventTypeCheckStatic           EventType = "check_static_layer"
	EventTypeCheckOCI              EventType = "check_oci_layer"
	EventTypeReverseProxy          EventType = "reverse_proxy"
	EventTypeTransferLayer         EventType = "transfer_layer_tcp"
)

type EventStatus string

const (
	// Normal type of event
	Normal EventStatus = "Normal"
	// Warning type of event
	Warning EventStatus = "Warning"
)

// Event represents a single recorded operation.
type Event struct {
	Type        EventType              `json:"type"`
	Timestamp   time.Time              `json:"timestamp"`
	RequestID   string                 `json:"requestID,omitempty"`
	EventStatus EventStatus            `json:"eventStatus,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Message     string                 `json:"message,omitempty"`
}

// Recorder records events in a bounded in-memory buffer.
// Optionally writes each event to a rotating file asynchronously when InitEventFile was called.
// When event file is enabled, List() reads from the file(s) so that data survives restarts.
type Recorder struct {
	mu         sync.RWMutex
	events     []Event
	size       int
	next       int
	count      int
	fileCh     chan Event // nil when file disabled; buffered for async write
	fileWg     sync.WaitGroup
	fileClosed atomic.Bool

	subsMu sync.RWMutex
	subs   []chan Event // buffered channels for follow mode; each has cap 256

	eventFileMu         sync.RWMutex
	eventFilePath       string // set when InitEventFile is called; used by List() to read from file
	eventFileMaxBackups int    // number of rotated backups to consider when reading
}

// Global returns the global recorder instance (singleton).
var Global = New(DefaultBufferSize)

// New creates a recorder that keeps at most size events (ring buffer).
func New(size int) *Recorder {
	if size <= 0 {
		size = DefaultBufferSize
	}
	return &Recorder{
		events: make([]Event, size),
		size:   size,
	}
}

// InitEventFile enables async writing of events to a rotating file at eventFile.
// maxSizeMB is the max size in megabytes before rotation (e.g. 1024 for 1GB);
// maxBackups is the number of rotated files to keep (e.g. 5).
// If eventFile is empty, file writing is disabled. Directory is created if needed.
// Record() never blocks on disk I/O; when the write buffer is full, file writes are dropped (in-memory ring buffer is still updated).
func (r *Recorder) InitEventFile(eventFile string, maxSizeMB, maxBackups int) error {
	if eventFile == "" {
		return nil
	}
	if maxSizeMB <= 0 {
		maxSizeMB = DefaultEventFileMaxSizeMB
	}
	if maxBackups <= 0 {
		maxBackups = DefaultEventFileMaxBackups
	}
	dir := filepath.Dir(eventFile)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	lj := &lumberjack.Logger{
		Filename:   eventFile,
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		Compress:   false,
	}
	bw := bufio.NewWriterSize(lj, 64*1024)
	ch := make(chan Event, eventFileChanCap)
	r.fileCh = ch
	r.eventFileMu.Lock()
	r.eventFilePath = eventFile
	r.eventFileMaxBackups = maxBackups
	r.eventFileMu.Unlock()
	r.fileWg.Add(1)
	go r.runFileWriter(bw, ch)
	return nil
}

// runFileWriter reads events from ch, marshals to JSON lines, and writes to w. Flushes periodically.
func (r *Recorder) runFileWriter(w *bufio.Writer, ch <-chan Event) {
	defer r.fileWg.Done()
	tick := time.NewTicker(eventFileFlushInterval)
	defer tick.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				_ = w.Flush()
				return
			}
			raw, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			_, _ = w.Write(raw)
			_, _ = w.Write([]byte{'\n'})
		case <-tick.C:
			_ = w.Flush()
		}
	}
}

// CloseEventFile stops the async file writer and flushes remaining events. Idempotent.
// Call during shutdown to avoid losing buffered events. No Record() should be called after this.
func (r *Recorder) CloseEventFile() {
	if r.fileClosed.Swap(true) {
		return
	}
	ch := r.fileCh
	if ch == nil {
		return
	}
	r.fileCh = nil // ensure new Record() see nil and skip
	close(ch)
	r.fileWg.Wait()
}

// Subscribe returns a channel that receives a copy of each new event from now on.
// Buffer size is 256; if the subscriber does not drain in time, new events are dropped for that subscriber.
// The caller must call the returned unsub function when done to close the channel and stop receiving.
func (r *Recorder) Subscribe() (ch <-chan Event, unsub func()) {
	c := make(chan Event, 256)
	r.subsMu.Lock()
	r.subs = append(r.subs, c)
	r.subsMu.Unlock()
	return c, func() {
		r.subsMu.Lock()
		defer r.subsMu.Unlock()
		for i, sub := range r.subs {
			if sub == c {
				r.subs = append(r.subs[:i], r.subs[i+1:]...)
				close(c)
				return
			}
		}
	}
}

// Record appends one event. If the buffer is full, the oldest event is overwritten.
// When event file is enabled, the event is enqueued for async write; Record() does not wait for disk.
// If the write queue is full, the event is still kept in the in-memory ring buffer but may not be written to file.
func (r *Recorder) Record(ctx context.Context, ev Event) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	ev.RequestID = logger.GetContextField(ctx, common.RequestIDHeaderKey)
	r.mu.Lock()
	r.events[r.next] = ev
	r.next = (r.next + 1) % r.size
	if r.count < r.size {
		r.count++
	}
	r.mu.Unlock()

	ch := r.fileCh
	if ch != nil && !r.fileClosed.Load() {
		select {
		case ch <- ev:
		default:
			// Queue full; drop file write to avoid blocking the caller. Ring buffer already has the event.
		}
	}

	// Notify follow subscribers (non-blocking)
	r.subsMu.RLock()
	subs := make([]chan Event, len(r.subs))
	copy(subs, r.subs)
	r.subsMu.RUnlock()
	for _, sub := range subs {
		select {
		case sub <- ev:
		default:
			// Subscriber slow; drop for this subscriber
		}
	}
}

// listFromFile reads events from the event file(s) in chronological order and returns the last limit events.
// File order: eventFile.MaxBackups (oldest), ..., eventFile.1, eventFile (newest). Skips unreadable or invalid lines.
func (r *Recorder) listFromFile(eventFile string, maxBackups, limit int, query []string, startTime *time.Time) []Event {
	if limit <= 0 {
		limit = 100
	}
	var events []Event
	// Build list of paths from oldest to newest: eventFile.5, eventFile.4, ..., eventFile.1, eventFile
	for i := maxBackups; i >= 1; i-- {
		path := eventFile + "." + strconv.Itoa(i)
		r.readEventsFromPath(path, &events, limit, query, startTime)
	}
	r.readEventsFromPath(eventFile, &events, limit, query, startTime)
	if len(events) == 0 {
		return nil
	}
	// Keep only the last limit events (we may have read more)
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events
}

// readEventsFromPath appends events from path (JSONL) into events, keeping at most limit in the sliding window.
func (r *Recorder) readEventsFromPath(path string, events *[]Event, limit int, query []string, startTime *time.Time) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// Increase buffer for long lines (e.g. large message)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if len(query) != 0 {
			matched := false
			str := utils.BytesToString(line)
			for i := range query {
				if strings.Contains(str, query[i]) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if startTime != nil {
			if ev.Timestamp.Before(*startTime) {
				continue
			}
		}
		*events = append(*events, ev)
		if len(*events) > limit {
			*events = (*events)[1:]
		}
	}
}

// List returns the most recent events, up to limit. Oldest of the returned set is first.
// If limit <= 0, default 100 is used.
// When event file is enabled (InitEventFile was called), List reads from the file(s) so data survives restarts.
// Otherwise List reads from the in-memory ring buffer.
func (r *Recorder) List(limit int, query []string, startTime *time.Time) []Event {
	if limit <= 0 {
		limit = 100
	}
	r.eventFileMu.RLock()
	eventFile := r.eventFilePath
	maxBackups := r.eventFileMaxBackups
	r.eventFileMu.RUnlock()

	if eventFile != "" {
		return r.listFromFile(eventFile, maxBackups, limit, query, startTime)
	}

	// No event file configured: read from in-memory ring buffer
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := r.count
	if n > limit {
		n = limit
	}
	if n == 0 {
		return nil
	}
	out := make([]Event, n)
	start := 0
	if r.count == r.size {
		start = (r.next - r.count + r.size) % r.size
	}
	for i := 0; i < n; i++ {
		idx := (start + i) % r.size
		out[i] = r.events[idx]
	}
	return out
}
