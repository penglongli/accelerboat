// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/customapi"
)

const (
	RequestIDHeaderKey = "X-Request-ID"
)

func completeRequestID(req *http.Request) (context.Context, string) {
	requestID := req.Header.Get(RequestIDHeaderKey)
	if requestID == "" {
		requestID = uuid.New().String()
	}
	reqCtx := logger.WithContextFields(req.Context(), RequestIDHeaderKey, requestID)
	return reqCtx, requestID
}

func CommonMiddleware() func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		reqCtx, requestID := completeRequestID(ctx.Request)
		ctx.Request = ctx.Request.WithContext(reqCtx)
		ctx.Writer.Header().Set(RequestIDHeaderKey, requestID)
		ctx.Request.Header.Set(RequestIDHeaderKey, requestID)
		req := ctx.Request
		if !strings.Contains(req.RequestURI, customapi.APIRecorder) {
			logger.InfoContextf(reqCtx, "received request: %s, %s%s", req.Method, req.Host, req.URL.String())
		}
		ctx.Next()
	}
}
