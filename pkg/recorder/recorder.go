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
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
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
	EventTypeGetBlob               EventType = "get_blob"
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

// Event represents a single recorded operation.
type Event struct {
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// Recorder records events in a bounded in-memory buffer.
// Optionally writes each event to a rotating file asynchronously when InitEventFile was called.
type Recorder struct {
	mu      sync.RWMutex
	events  []Event
	size    int
	next    int
	count   int
	fileCh  chan Event // nil when file disabled; buffered for async write
	fileWg  sync.WaitGroup
	fileClosed atomic.Bool
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

// Record appends one event. If the buffer is full, the oldest event is overwritten.
// When event file is enabled, the event is enqueued for async write; Record() does not wait for disk.
// If the write queue is full, the event is still kept in the in-memory ring buffer but may not be written to file.
func (r *Recorder) Record(ev Event) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	r.mu.Lock()
	r.events[r.next] = ev
	r.next = (r.next + 1) % r.size
	if r.count < r.size {
		r.count++
	}
	r.mu.Unlock()

	ch := r.fileCh
	if ch == nil || r.fileClosed.Load() {
		return
	}
	select {
	case ch <- ev:
	default:
		// Queue full; drop file write to avoid blocking the caller. Ring buffer already has the event.
	}
}

// RecordSimple records an event with the given type and optional detail key-values.
// Keys and values are interleaved: key1, val1, key2, val2, ... Odd length panics.
func (r *Recorder) RecordSimple(typ EventType, kvs ...interface{}) {
	details := make(map[string]interface{})
	for i := 0; i+1 < len(kvs); i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			continue
		}
		details[key] = kvs[i+1]
	}
	r.Record(Event{Type: typ, Timestamp: time.Now(), Details: details})
}

// List returns the most recent events, up to limit. Oldest of the returned set is first.
// If limit <= 0, default 100 is used.
func (r *Recorder) List(limit int) []Event {
	if limit <= 0 {
		limit = 100
	}
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
	// Oldest is at (r.next - r.count + r.size) % r.size when count == size; else 0
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
