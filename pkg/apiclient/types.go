// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package apiclient

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/penglongli/accelerboat/pkg/logger"
)

const (
	RequestIDHeaderKey = "X-Request-ID"
)

// SetContext set the request context
func SetContext(req *http.Request) *http.Request {
	requestID := req.Header.Get(RequestIDHeaderKey)
	if requestID == "" {
		requestID = uuid.New().String()
	}
	reqCtx := logger.WithContextFields(req.Context(), RequestIDHeaderKey, requestID)
	return req.WithContext(reqCtx)
}
