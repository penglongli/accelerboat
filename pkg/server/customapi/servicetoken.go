// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/utils/httputils"
)

func buildAuthTokenKey(originalHost, service, scope string) string {
	return fmt.Sprintf("%s,%s,%s", originalHost, service, scope)
}

func getServiceTokenWithCheck(ctx context.Context, req *apitypes.GetServiceTokenRequest) (
	*apitypes.RegistryAuthToken, error) {
	respBody, err := httputils.SendHTTPRequest(ctx, &httputils.HTTPRequest{
		Url:         req.ServiceTokenUrl,
		Method:      http.MethodGet,
		HeaderMulti: req.Headers,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get service token")
	}
	token := &apitypes.RegistryAuthToken{}
	if err = json.Unmarshal(respBody, token); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal service token(value: %s)", string(respBody))
	}
	logger.InfoContextf(ctx, "get service token success")

	scopeArr := strings.Split(req.Scope, ":")
	if len(scopeArr) != 3 && scopeArr[0] != "repository" {
		logger.WarnContextf(ctx, "scope '%s' not repository, noneed check the token", req.Scope)
		return token, nil
	}
	checkResp, err := httputils.SendHTTPRequestOnlyResponse(ctx, &httputils.HTTPRequest{
		// We use the `latest` tag for validation, regardless of whether it actually has `latest`,
		// because we only use it to determine if the token is valid.
		Url:    fmt.Sprintf("https://%s/v2/%s/manifests/latest", req.OriginalHost, scopeArr[1]),
		Method: http.MethodHead,
		Header: map[string]string{
			"Authorization": fmt.Sprintf("Bearer %s", token.Token),
		},
	})
	if err != nil {
		return token, errors.Wrapf(err, "failed to check service token")
	}
	defer checkResp.Body.Close()
	if checkResp.StatusCode != http.StatusUnauthorized && checkResp.StatusCode < 500 {
		logger.InfoContextf(ctx, "check service token success")
		return token, nil
	}
	return token, fmt.Errorf("check service token failed, status code: %d", checkResp.StatusCode)
}

func (h *CustomHandler) saveAuthToken(authKey string, authToken *apitypes.RegistryAuthToken) {
	var expire time.Duration
	if authToken.ExpiresIn > 180 {
		expire = 180 * time.Second
	} else {
		expire = time.Duration(authToken.ExpiresIn) * time.Second
	}
	h.authTokens.Set(authKey, authToken, expire)
	// Shorten the expiration time by 60 seconds to prevent clients from sending tokens that are nearing expiration.
	// Because our intermediate links have some latency, if a token nearing expiration is on our link,
	// it's easy for delays to cause a 401 Unauthorized error.
	if authToken.ExpiresIn-60 > 180 {
		authToken.ExpiresIn = authToken.ExpiresIn - 60
	}
	logger.Infof("cache authkey %s set value %s", authKey, authToken.Token)
}

// GetServiceToken obtains a registry auth token from upstream and returns it (cached by originalHost, service, scope).
func (h *CustomHandler) GetServiceToken(c *gin.Context) (interface{}, error) {
	req := &apitypes.GetServiceTokenRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	authKey := buildAuthTokenKey(req.OriginalHost, req.Service, req.Scope)
	ctx := c.Request.Context()
	h.authLock.Lock(ctx, authKey)
	defer h.authLock.UnLock(ctx, authKey)

	auth, ok := h.authTokens.Get(authKey)
	if ok && auth != nil {
		return auth.(*apitypes.RegistryAuthToken), nil
	}
	logger.InfoContextf(ctx, "cache authkey: %s", authKey)

	delete(req.Headers, "Accept-Encoding")
	originalAuthToken, err := getServiceTokenWithCheck(ctx, req)
	if err == nil {
		h.saveAuthToken(authKey, originalAuthToken)
		return originalAuthToken, nil
	}
	var legalUsers []*options.RegistryAuth
	registry := h.op.FilterRegistryMappingByOriginal(req.OriginalHost)
	if registry != nil {
		legalUsers = registry.LegalUsers
	}
	if len(legalUsers) == 0 {
		if originalAuthToken != nil {
			return originalAuthToken, nil
		}
		return nil, err
	}

	logger.WarnContextf(ctx, "get service token use original request failed: %s, "+
		"will retry with configured auths", err.Error())
	for i, user := range legalUsers {
		req.Headers["Authorization"] = []string{fmt.Sprintf("Basic %s", base64.StdEncoding.
			EncodeToString([]byte(fmt.Sprintf("%s:%s", user.Username, user.Password))))}
		var authToken *apitypes.RegistryAuthToken
		if authToken, err = getServiceTokenWithCheck(ctx, req); err != nil {
			logger.WarnContextf(ctx, "get service token with user[%d] '%s' failed: %s",
				i, user.Username, err.Error())
			continue
		}
		h.saveAuthToken(authKey, authToken)
		return authToken, nil
	}
	logger.WarnContextf(ctx, "get service token still failed after retry with configured auths")
	if originalAuthToken != nil {
		return originalAuthToken, nil
	}
	return nil, fmt.Errorf("get service token failed")
}
