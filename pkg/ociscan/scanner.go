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

package ociscan

import (
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/platforms"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/metrics"
	"github.com/penglongli/accelerboat/pkg/store"
	"github.com/penglongli/accelerboat/pkg/utils"
)

// ScanHandler defines the handler for scan oci
type ScanHandler struct {
	op         *options.AccelerBoatOption
	cacheStore store.CacheStore

	cc               *containerdChecker
	containerdLayers map[string]string
}

// NewScanHandler create scan handler instance
func NewScanHandler() *ScanHandler {
	op := options.GlobalOptions()
	return &ScanHandler{
		op:               op,
		cacheStore:       store.GlobalRedisStore(),
		containerdLayers: make(map[string]string),
	}
}

// Init the scan handler
func (s *ScanHandler) Init() error {
	s.cc = s.initContainerdChecker()
	s.reportOCILayers(context.Background())
	return nil
}

// TickerReport ticker report oci layers
func (s *ScanHandler) TickerReport(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.reportOCILayers(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// reportOCILayers report docker and containerd oci-layers
func (s *ScanHandler) reportOCILayers(ctx context.Context) {
	if s.cc != nil {
		layers := s.cc.Parse(ctx)
		for k, v := range layers {
			if err := s.cacheStore.SaveOCILayer(ctx, store.CONTAINERD, k, v); err != nil {
				logger.Errorf("save oci layer '%s' failed: %s", k, err.Error())
			}
		}
		for k := range s.containerdLayers {
			if _, ok := layers[k]; ok {
				continue
			}
			if err := s.cacheStore.DeleteOCILayer(ctx, store.CONTAINERD, k); err != nil {
				logger.Errorf("delete oci layer '%s' failed: %s", k, err.Error())
			} else {
				logger.Infof("delete oci layer '%s' success", k)
			}
		}
	}
}

// GenerateLayer generate layers to target file with oci api
func (s *ScanHandler) GenerateLayer(ctx context.Context, ociType string, layer string) (string, error) {
	var result string
	var err error
	switch store.LayerType(ociType) {
	case store.CONTAINERD:
		if s.cc == nil {
			return "", errors.Errorf("copy containerd layer no handler")
		}
		result, err = s.handleContainerdCopy(ctx, layer)
	default:
		return "", errors.Errorf("layer path 'type(%s), file(%s)' is unknown", ociType, layer)
	}
	if err != nil {
		metrics.RecordError(metrics.ComponentOCIScan, "generate_layer")
		return "", errors.Wrapf(err, "generate '%s' oci-layer failed", ociType)
	}
	return result, nil
}

// handleContainerdCopy handle containerd copy
func (s *ScanHandler) handleContainerdCopy(ctx context.Context, layer string) (string, error) {
	fullDigest := "sha256:" + strings.TrimPrefix(layer, "sha256:")
	layerDigest := digest.Digest(fullDigest)
	nsCtx := namespaces.WithNamespace(ctx, "k8s.io")
	if _, err := s.cc.Client.ContentStore().Info(nsCtx, layerDigest); err != nil {
		if errdefs.IsNotFound(err) {
			return "", errors.Wrapf(err, "containerd get layer '%s' not found", layerDigest)
		}
		return "", errors.Wrapf(err, "containerd get layer info failed")
	}

	ra, err := s.cc.Client.ContentStore().ReaderAt(nsCtx, ocispec.Descriptor{Digest: digest.Digest(fullDigest)})
	if err != nil {
		return "", errors.Wrapf(err, "containerd read digest failed")
	}
	defer ra.Close()
	logger.InfoContextf(ctx, "layer-containerd read layer '%s' sucess", fullDigest)

	// Layer file name without "sha256:" prefix (same as LayerFileName in utils).
	layerFileName := utils.LayerFileName(fullDigest)
	reader := content.NewReader(ra)
	targetFile := path.Join(s.op.StorageConfig.DownloadPath, layerFileName)
	_ = os.RemoveAll(targetFile)
	dstFile, err := os.Create(targetFile)
	if err != nil {
		return "", errors.Wrapf(err, "containerd create layer '%s' failed", targetFile)
	}
	defer dstFile.Close()
	if _, err = io.Copy(dstFile, reader); err != nil {
		return "", errors.Wrapf(err, "containerd copy layer '%s' failed", targetFile)
	}
	result := path.Join(s.op.StorageConfig.OCIPath, layerFileName)
	if err = os.Rename(targetFile, result); err != nil {
		return "", errors.Wrapf(err, "rename '%s' to '%s' failed", targetFile, result)
	}
	return result, nil
}

// ImageLayerInfo holds digest, size and optional local path for an OCI image layer.
type ImageLayerInfo struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	LocalPath string `json:"localPath,omitempty"`
}

// ImageInfo holds image name, target digest and layer list for a managed OCI image.
type ImageInfo struct {
	Name   string           `json:"name"`
	Target string           `json:"target"`
	Layers []ImageLayerInfo `json:"layers"`
}

// OCIPathLayerInfo holds digest, size and path for a layer file under OCIPath.
type OCIPathLayerInfo struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	Path   string `json:"path"`
}

// ListManagedImages returns OCI images from containerd (if enabled) with each image's layer info.
// ociPath is the directory to check for existing layer files (e.g. OCIPath); may be empty.
func (s *ScanHandler) ListManagedImages(ctx context.Context, ociPath string) ([]ImageInfo, error) {
	if s.cc == nil {
		return nil, nil
	}
	nsCtx := namespaces.WithNamespace(ctx, "k8s.io")
	client := s.cc.Client
	cs := client.ContentStore()
	platform := platforms.Default()

	imageList, err := client.ListImages(nsCtx)
	if err != nil {
		return nil, errors.Wrap(err, "list containerd images failed")
	}
	// Build set of layer digests that have a file under ociPath (for LocalPath).
	ociPathLayers := make(map[string]string)
	if ociPath != "" {
		_ = filepath.WalkDir(ociPath, func(entryPath string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(de.Name(), ".tar.gzip") {
				return nil
			}
			name := strings.TrimSuffix(de.Name(), ".tar.gzip")
			// File names are without "sha256:" prefix; support legacy names with prefix.
			d := "sha256:" + strings.TrimPrefix(name, "sha256:")
			ociPathLayers[d] = entryPath
			return nil
		})
	}

	out := make([]ImageInfo, 0, len(imageList))
	for _, img := range imageList {
		target := img.Target()
		manifest, err := images.Manifest(nsCtx, cs, target, platform)
		if err != nil {
			logger.WarnContextf(ctx, "get manifest for image %s failed: %s", img.Name(), err.Error())
			continue
		}
		layers := make([]ImageLayerInfo, 0, len(manifest.Layers))
		for _, desc := range manifest.Layers {
			d := desc.Digest.String()
			li := ImageLayerInfo{Digest: d, Size: desc.Size}
			if p, ok := ociPathLayers[d]; ok {
				li.LocalPath = p
			}
			layers = append(layers, li)
		}
		out = append(out, ImageInfo{
			Name:   img.Name(),
			Target: target.Digest.String(),
			Layers: layers,
		})
	}
	return out, nil
}

// ListOCIPathLayers returns layer files under ociPath with digest and size.
func ListOCIPathLayers(ociPath string) ([]OCIPathLayerInfo, error) {
	if ociPath == "" {
		return nil, nil
	}
	var out []OCIPathLayerInfo
	err := filepath.WalkDir(ociPath, func(entryPath string, de os.DirEntry, err error) error {
		if err != nil || de.IsDir() || !strings.HasSuffix(de.Name(), ".tar.gzip") {
			return nil
		}
		info, err := de.Info()
		if err != nil {
			return nil
		}
		name := strings.TrimSuffix(de.Name(), ".tar.gzip")
		// File names are without "sha256:" prefix; support legacy names with prefix.
		d := "sha256:" + strings.TrimPrefix(name, "sha256:")
		out = append(out, OCIPathLayerInfo{Digest: d, Size: info.Size(), Path: entryPath})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// containerdChecker defines the containerd checker
type containerdChecker struct {
	Client *containerd.Client
}

// initContainerdChecker init the containerd checker
func (s *ScanHandler) initContainerdChecker() *containerdChecker {
	if !s.op.EnableContainerd {
		return nil
	}
	cc, err := containerd.New("/run/containerd/containerd.sock")
	if err != nil {
		logger.Errorf("ignore containerd. init containerd client failed: %s", err.Error())
		return nil
	}
	logger.Infof("init containerd client success")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var vs containerd.Version
	if vs, err = cc.Version(ctx); err != nil {
		logger.Warnf("ignore containerd. get containerd version failed: %s", err.Error())
	} else {
		logger.Infof("init containerd get version sucees: %s", vs.Version)
	}
	return &containerdChecker{
		Client: cc,
	}
}

// Parse the layers from containerd
func (c *containerdChecker) Parse(ctx context.Context) map[string]string {
	nsCtx := namespaces.WithNamespace(ctx, "k8s.io")
	result := make(map[string]string)
	err := c.Client.ContentStore().Walk(nsCtx, func(info content.Info) error {
		digestStr := strings.TrimPrefix(info.Digest.String(), "sha256:")
		result[digestStr] = digestStr
		return nil
	})
	if err != nil {
		logger.ErrorContextf(ctx, "containerd walk get digests failed: %s", err.Error())
	}
	return result
}
