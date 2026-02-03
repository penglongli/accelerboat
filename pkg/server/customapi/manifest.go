// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/utils/httputils"
)

func buildManifestKey(originalHost, repo, tag string) string {
	return fmt.Sprintf("%s,%s,%s", originalHost, repo, tag)
}

func (h *CustomHandler) RegistryHeadManifest(c *gin.Context) (interface{}, error) {
	req := &apitypes.HeadManifestRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	lockKey := buildManifestKey(req.OriginalHost, req.Repo, req.Tag)
	ctx := c.Request.Context()
	h.headManifestLock.Lock(ctx, lockKey)
	defer h.headManifestLock.UnLock(ctx, lockKey)

	v, ok := h.headManifests.Get(lockKey)
	if ok && v != nil {
		return &apitypes.HeadManifestResponse{Headers: v.(map[string][]string)}, nil
	}
	logger.InfoContextf(ctx, "handling head image manifest request")
	resp, _, err := httputils.SendHTTPRequestReturnResponse(ctx, &httputils.HTTPRequest{
		Url:         fmt.Sprintf("https://%s%s", req.OriginalHost, req.HeadManifestUrl),
		Method:      http.MethodHead,
		HeaderMulti: req.Headers,
	})
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for k, v := range resp.Header {
		result[k] = v
	}
	h.headManifests.Set(lockKey, result, 10*time.Second)
	return &apitypes.HeadManifestResponse{Headers: result}, nil
}

func (h *CustomHandler) RegistryGetManifest(c *gin.Context) (interface{}, error) {
	req := &apitypes.GetManifestRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	lockKey := buildManifestKey(req.OriginalHost, req.Repo, req.Tag)
	ctx := c.Request.Context()
	h.getManifestLock.Lock(ctx, lockKey)
	defer h.getManifestLock.UnLock(ctx, lockKey)

	v, ok := h.manifests.Get(lockKey)
	if ok && v != nil {
		return v.(string), nil
	}
	logger.InfoContextf(ctx, "handling get image manifest request")
	respBody, err := httputils.SendHTTPRequest(ctx, &httputils.HTTPRequest{
		Url:         fmt.Sprintf("https://%s%s", req.OriginalHost, req.ManifestUrl),
		Method:      http.MethodGet,
		HeaderMulti: req.Headers,
	})
	if err != nil {
		return nil, err
	}
	manifest := string(respBody)
	h.manifests.Set(lockKey, manifest, 10*time.Second)
	return manifest, nil
}
