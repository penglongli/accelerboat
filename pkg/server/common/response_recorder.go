// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package common

import (
	"net/http"
)

// ResponseRecorder wraps http.ResponseWriter to capture status code for metrics.
type ResponseRecorder struct {
	http.ResponseWriter
	status int
}

// NewResponseRecorder returns a new ResponseRecorder.
func NewResponseRecorder(w http.ResponseWriter) *ResponseRecorder {
	return &ResponseRecorder{ResponseWriter: w, status: http.StatusOK}
}

// Status returns the captured HTTP status code.
func (r *ResponseRecorder) Status() int {
	return r.status
}

func (r *ResponseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher so that streaming handlers (e.g. /customapi/recorder with follow=true)
// can flush chunked data to the client. If the underlying ResponseWriter supports Flusher, it is delegated.
func (r *ResponseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
