// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/common"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
)

func completeRequestID(req *http.Request) (context.Context, string) {
	requestID := req.Header.Get(common.RequestIDHeaderKey)
	if requestID == "" {
		requestID = uuid.New().String()
	}
	reqCtx := logger.WithContextFields(req.Context(), common.RequestIDHeaderKey, requestID)
	return reqCtx, requestID
}

func GinMiddleware() func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		reqCtx, requestID := completeRequestID(ctx.Request)
		ctx.Request = ctx.Request.WithContext(reqCtx)
		ctx.Writer.Header().Set(common.RequestIDHeaderKey, requestID)
		ctx.Request.Header.Set(common.RequestIDHeaderKey, requestID)
		req := ctx.Request
		if _, ok := apitypes.NotPrintLog[req.RequestURI]; !ok {
			logger.InfoContextf(reqCtx, "received request: %s, %s%s", req.Method, req.Host, req.URL.String())
		}
		ctx.Next()
	}
}

func GeneralMiddleware(rw http.ResponseWriter, req *http.Request) *http.Request {
	reqCtx, requestID := completeRequestID(req)
	newReq := req.WithContext(reqCtx)
	rw.Header().Set(common.RequestIDHeaderKey, requestID)
	newReq.Header.Set(common.RequestIDHeaderKey, requestID)
	logger.InfoContextf(reqCtx, "received request: %s, %s%s", req.Method, req.Host, req.URL.String())
	return newReq
}
