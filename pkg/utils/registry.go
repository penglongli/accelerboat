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

package utils

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// ChangeAuthenticateHeader rewrites Www-Authenticate realm to the proxy's service/token URL.
// TODO: refactor as needed.
func ChangeAuthenticateHeader(resp *http.Response, proxyHost string) {
	v := resp.Header.Get("Www-Authenticate")
	if v == "" {
		return
	}
	realm, scope, service := ParseAuthRequest(v)
	if realm == "" {
		return
	}
	realm = fmt.Sprintf("%s/service/token", proxyHost)
	newV := BuildAuthenticateHeader(realm, scope, service)
	resp.Header.Set("Www-Authenticate", newV)
}

func BuildAuthenticateHeader(realm, service, scope string) string {
	result := make([]string, 0)
	result = append(result, fmt.Sprintf(`Bearer realm="%s"`, realm))
	if service != "" {
		result = append(result, fmt.Sprintf(`service="%s"`, service))
	}
	if scope != "" {
		result = append(result, fmt.Sprintf(`scope="%s"`, scope))
	}
	return strings.Join(result, ",")
}

// ParseAuthRequest parse the auth request header
func ParseAuthRequest(authHeader string) (string, string, string) {
	authenticate := strings.TrimSpace(authHeader)
	if authenticate == "" || !strings.HasPrefix(authenticate, "Bearer realm") {
		return "", "", ""
	}
	return parseAuthenticateHeader(authenticate)
}

var (
	realmRegex   = regexp.MustCompile(`realm="(.*?)"`)
	serviceRegex = regexp.MustCompile(`service="(.*?)"`)
	scopeRegex   = regexp.MustCompile(`scope="(.*?)"`)
)

func parseAuthenticateHeader(header string) (string, string, string) {
	realm := realmRegex.FindStringSubmatch(header)
	service := serviceRegex.FindStringSubmatch(header)
	scope := scopeRegex.FindStringSubmatch(header)

	var realmValue, serviceValue, scopeValue string
	if len(realm) > 1 {
		realmValue = realm[1]
	}
	if len(service) > 1 {
		serviceValue = service[1]
	}
	if len(scope) > 1 {
		scopeValue = scope[1]
	}
	return realmValue, serviceValue, scopeValue
}

var (
	manifestUriRegexp = regexp.MustCompile(`^/v[1-2]/(.*)/manifests/(.*)`)
	blobUriRegexp     = regexp.MustCompile(`^/v[1-2]/(.*)/blobs/sha256:([a-z0-9A-Z]{64})$`)
)

func IsServiceToken(r *http.Request) (string, string, bool) {
	if r.Method != http.MethodGet {
		return "", "", false
	}
	if r.URL.Path != "/service/token" {
		return "", "", false
	}
	service := r.URL.Query().Get("service")
	scope := r.URL.Query().Get("scope")
	return service, scope, true
}

func IsHeadImageDigest(r *http.Request) (string, string, bool) {
	if r.Method != http.MethodHead {
		return "", "", false
	}
	if r.URL == nil {
		return "", "", false
	}
	result := manifestUriRegexp.FindStringSubmatch(r.URL.Path)
	if len(result) != 3 {
		return "", "", false
	}
	repo := result[1]
	tag := result[2]
	return repo, tag, true
}

// IsManifestGet used to check the uri whether is manifest-get
// e.p: /v2/tencentmirrors/centos/manifests/7 => tencentmirrors/centos, 7, nil
func IsManifestGet(r *http.Request) (string, string, bool) {
	if r.Method != http.MethodGet {
		return "", "", false
	}
	if r.URL == nil {
		return "", "", false
	}
	result := manifestUriRegexp.FindStringSubmatch(r.URL.Path)
	if len(result) != 3 {
		return "", "", false
	}
	repo := result[1]
	tag := result[2]
	return repo, tag, true
}

// IsBlobGet used to check the uri whether is blob-download
// e.p: /v2/instantlinux/haproxy-keepalived/blobs/sha256:ec99f8b99825a742d50fb3ce173d291378a46ab54b8ef7dd75e5654e2a296e99
// => instantlinux/haproxy-keepalived, ec99f8b99825a742d50fb3ce173d291378a46ab54b8ef7dd75e5654e2a296e99
func IsBlobGet(url string) (string, string, bool) {
	result := blobUriRegexp.FindStringSubmatch(url)
	if len(result) != 3 {
		return "", "", false
	}
	repo := result[1]
	sha256 := result[2]
	return repo, sha256, true
}

// LayerFileName return layer name
func LayerFileName(digest string) string {
	digest = strings.TrimPrefix(digest, "sha256:")
	return digest + ".tar.gzip"
}
