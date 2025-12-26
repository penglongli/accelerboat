// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package requester

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server/common"
	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
	"github.com/penglongli/accelerboat/pkg/utils"
)

func commonHeaders(ctx context.Context) map[string]string {
	result := make(map[string]string)
	reqID := logger.GetContextField(ctx, common.RequestIDHeaderKey)
	if reqID != "" {
		result[common.RequestIDHeaderKey] = reqID
	}
	return result
}

// GetServiceToken get token from master
func GetServiceToken(ctx context.Context, req *apitypes.GetServiceTokenRequest) (string, string, error) {
	op := options.GlobalOptions()
	master := op.CurrentMaster()
	newCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := utils.SendHTTPRequest(newCtx, &utils.HTTPRequest{
		Url:    fmt.Sprintf("http://%s%s", master, apitypes.APIGetServiceToken),
		Method: http.MethodPost,
		Body:   req,
		Header: commonHeaders(ctx),
	})
	if err != nil {
		return master, "", errors.Wrapf(err, "get service-token failed")
	}
	token := strings.TrimSpace(string(body))
	if token == "" {
		return master, token, errors.New("empty token object")
	}
	return master, token, nil
}

func HeadManifest(ctx context.Context, req *apitypes.HeadManifestRequest) (map[string][]string, error) {
	op := options.GlobalOptions()
	master := op.CurrentMaster()
	newCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := utils.SendHTTPRequest(newCtx, &utils.HTTPRequest{
		Url:    fmt.Sprintf("http://%s%s", master, apitypes.APIHeadManifest),
		Method: http.MethodPost,
		Body:   req,
		Header: commonHeaders(ctx),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "head image digest failed")
	}
	resp := new(apitypes.HeadManifestResponse)
	if err = json.Unmarshal(body, resp); err != nil {
		return nil, errors.Wrapf(err, "head image digest unmarshal failed")
	}
	return resp.Headers, nil
}

// GetManifest get manifest from master
func GetManifest(ctx context.Context, req *apitypes.GetManifestRequest) (string, string, error) {
	op := options.GlobalOptions()
	master := op.CurrentMaster()
	newCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := utils.SendHTTPRequest(newCtx, &utils.HTTPRequest{
		Url:    fmt.Sprintf("http://%s%s", master, apitypes.APIGetManifest),
		Method: http.MethodPost,
		Body:   req,
		Header: commonHeaders(ctx),
	})
	if err != nil {
		return master, "", errors.Wrapf(err, "get manifest failed")
	}
	manifest := strings.TrimSpace(string(body))
	if manifest == "" {
		return master, manifest, errors.New("empty manifest")
	}
	return master, manifest, nil
}

// DownloadLayerFromMaster download layer from master
func DownloadLayerFromMaster(ctx context.Context, req *apitypes.DownloadLayerRequest, digest string) (
	*apitypes.DownloadLayerResponse, string, error) {
	op := options.GlobalOptions()
	master := op.CurrentMaster()
	body, err := utils.SendHTTPRequest(ctx, &utils.HTTPRequest{
		Url:    fmt.Sprintf("http://%s%s", master, apitypes.APIGetLayerInfo),
		Method: http.MethodPost,
		Body:   req,
	})
	if err != nil {
		return nil, master, errors.Wrapf(err, "get layer failed")
	}
	resp := new(apitypes.DownloadLayerResponse)
	if err = json.Unmarshal(body, resp); err != nil {
		return nil, master, errors.Wrapf(err, "unmarshal resp body failed")
	}
	return resp, master, nil
}

// CheckStaticLayer check static layer exist
func CheckStaticLayer(ctx context.Context, target string, req *apitypes.CheckStaticLayerRequest) (
	*apitypes.CheckStaticLayerResponse, error) {
	op := options.GlobalOptions()
	body, err := utils.SendHTTPRequest(ctx, &utils.HTTPRequest{
		Url:    fmt.Sprintf("http://%s:%d%s", target, op.HTTPPort, apitypes.APICheckStaticLayer), // nolint
		Method: http.MethodGet,
		Body:   req,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "check static-layer failed")
	}
	resp := new(apitypes.CheckStaticLayerResponse)
	if err = json.Unmarshal(body, resp); err != nil {
		return nil, errors.Wrapf(err, "unmarshal resp body failed")
	}
	return resp, nil
}

// CheckOCILayer check oci layer exist
func CheckOCILayer(ctx context.Context, target string, req *apitypes.CheckOCILayerRequest) (
	*apitypes.CheckOCILayerResponse, error) {
	op := options.GlobalOptions()
	body, err := utils.SendHTTPRequest(ctx, &utils.HTTPRequest{
		Url:    fmt.Sprintf("http://%s:%d%s", target, op.HTTPPort, apitypes.APICheckOCILayer), // nolint
		Method: http.MethodGet,
		Body:   req,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "check oci-layer failed")
	}
	resp := new(apitypes.CheckOCILayerResponse)
	if err = json.Unmarshal(body, resp); err != nil {
		return nil, errors.Wrapf(err, "unmarshal resp body failed")
	}
	return resp, nil
}

// DownloadLayerFromNode download layer from node
func DownloadLayerFromNode(ctx context.Context, target string, req *apitypes.DownloadLayerRequest) (
	*apitypes.DownloadLayerResponse, error) {
	body, err := utils.SendHTTPRequest(ctx, &utils.HTTPRequest{
		Url:    fmt.Sprintf("http://%s%s", target, apitypes.APIDownloadLayer), // nolint
		Method: http.MethodGet,
		Body:   req,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "download layer from node failed")
	}
	resp := new(apitypes.DownloadLayerResponse)
	if err = json.Unmarshal(body, resp); err != nil {
		return nil, errors.Wrapf(err, "unmarshal resp body failed")
	}
	return resp, nil
}
