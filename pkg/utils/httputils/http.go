/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package httputils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/moul/http2curl"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/common"
	"github.com/penglongli/accelerboat/pkg/utils"
)

// HTTPRequest defines the http request
type HTTPRequest struct {
	Url         string
	Method      string
	QueryParams map[string]string
	Body        interface{}
	Header      map[string]string
	HeaderMulti map[string][]string
}

// SendHTTPRequest the http request
func SendHTTPRequest(ctx context.Context, hr *HTTPRequest) ([]byte, error) {
	_, respBody, err := SendHTTPRequestReturnResponse(ctx, hr)
	return respBody, err
}

// SendHTTPRequestReturnResponse the http request return response
func SendHTTPRequestReturnResponse(ctx context.Context, hr *HTTPRequest) (*http.Response, []byte, error) {
	resp, err := SendHTTPRequestOnlyResponse(ctx, hr)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	var respBody []byte
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, errors.Wrap(err, "read response body failed")
	}
	if resp.StatusCode != http.StatusOK {
		return resp, nil, fmt.Errorf("http response %d: %s", resp.StatusCode,
			utils.BytesToString(respBody))
	}
	return resp, respBody, nil
}

func SendHTTPRequestOnlyResponse(ctx context.Context, hr *HTTPRequest) (*http.Response, error) {
	var req *http.Request
	var err error

	if !strings.Contains(hr.Url, "customapi") {
		logger.InfoContextf(ctx, "do request '%s, %s'", hr.Method, hr.Url)
	}
	if hr.Body != nil {
		var body []byte
		body, err = json.Marshal(hr.Body)
		if err != nil {
			return nil, errors.Wrapf(err, "marshal body failed")
		}
		req, err = http.NewRequestWithContext(ctx, hr.Method, hr.Url, bytes.NewBuffer(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, hr.Method, hr.Url, nil)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "create http request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hr.Header {
		req.Header.Set(k, v)
	}
	for k, v := range hr.HeaderMulti {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}
	requestID := logger.GetContextField(ctx, common.RequestIDHeaderKey)
	if requestID != "" {
		req.Header.Set(common.RequestIDHeaderKey, requestID)
	}

	if hr.QueryParams != nil {
		query := req.URL.Query()
		for k, v := range hr.QueryParams {
			query.Set(k, v)
		}
		req.URL.RawQuery = query.Encode()
	}
	command, _ := http2curl.GetCurlCommand(req)
	logger.V(3).InfoContextf(ctx, "Request: %s", command.String())

	var resp *http.Response
	httpClient := &http.Client{}
	if !strings.Contains(hr.Url, "customapi") {
		httpClient.Transport = options.GlobalOptions().HTTPProxyTransport()
	} else {
		httpClient.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialTLSContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				dialer := &net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}
				return dialer.DialContext(ctx, network, addr)
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	}
	for i := 0; i < 10; i++ {
		resp, err = httpClient.Do(req)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "context canceled") ||
			strings.Contains(err.Error(), "i/o timeout") {
			return nil, err
		}
		logger.WarnContextf(ctx, "do request '%s, %s' failed(retry=%d): %s", req.Method,
			req.URL.String(), i, err.Error())
		time.Sleep(time.Second)
	}
	if err != nil {
		return nil, errors.Wrap(err, "http request failed")
	}
	if resp == nil {
		return nil, errors.New("http response is nil")
	}
	logger.V(3).InfoContextf(ctx, "Response Code: %d", resp.StatusCode)
	logger.V(3).Infof("Response Headers:")
	for k, v := range resp.Header {
		logger.V(3).Infof("    %s: %s", k, strings.Join(v, ", "))
	}
	return resp, nil
}
