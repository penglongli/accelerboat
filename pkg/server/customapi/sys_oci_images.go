// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/pkg/ociscan"
	"github.com/penglongli/accelerboat/pkg/utils/formatutils"
)

// OCIImagesResponse is the response for GET /customapi/oci-images.
type OCIImagesResponse struct {
	ContainerdEnabled bool                      `json:"containerdEnabled"`
	Images            []ociscan.ImageInfo      `json:"images"`
	OCIPathLayers     []ociscan.OCIPathLayerInfo `json:"ociPathLayers"`
}

// OCIImages returns OCI managed images (from containerd) and layer files under OCIPath.
// Use query output=json for JSON; otherwise returns human-readable text.
func (h *CustomHandler) OCIImages(c *gin.Context) (interface{}, string, error) {
	ctx := c.Request.Context()
	op := h.op
	ociPath := op.StorageConfig.OCIPath

	imagesList, err := h.ociScanner.ListManagedImages(ctx, ociPath)
	if err != nil {
		return nil, "", errors.Wrap(err, "list managed images failed")
	}
	ociPathLayers, err := ociscan.ListOCIPathLayers(ociPath)
	if err != nil {
		return nil, "", errors.Wrap(err, "list oci path layers failed")
	}
	resp := &OCIImagesResponse{
		ContainerdEnabled: op.EnableContainerd,
		Images:            imagesList,
		OCIPathLayers:     ociPathLayers,
	}
	text := formatOCIImagesText(resp, ociPath)
	return resp, text, nil
}

func formatOCIImagesText(resp *OCIImagesResponse, ociPath string) string {
	var b strings.Builder
	b.WriteString("=== OCI Managed Images ===\n\n")
	b.WriteString(fmt.Sprintf("Containerd:  %s\n", formatBool(resp.ContainerdEnabled)))
	b.WriteString(fmt.Sprintf("OCIPath:     %s\n\n", ociPath))

	if len(resp.Images) == 0 {
		b.WriteString("Images: (none from containerd)\n\n")
	} else {
		b.WriteString(fmt.Sprintf("Images: %d\n\n", len(resp.Images)))
		for i, img := range resp.Images {
			b.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, img.Name))
			b.WriteString(fmt.Sprintf("       target: %s\n", img.Target))
			b.WriteString(fmt.Sprintf("       layers: %d\n", len(img.Layers)))
			for j, layer := range img.Layers {
				line := fmt.Sprintf("         %d. %s  %s", j+1, layer.Digest, formatutils.FormatSize(layer.Size))
				if layer.LocalPath != "" {
					line += "  [cached]"
				}
				b.WriteString(line + "\n")
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("--- OCIPath layer files ---\n\n")
	if len(resp.OCIPathLayers) == 0 {
		b.WriteString("(none)\n")
	} else {
		b.WriteString(fmt.Sprintf("%d layer file(s):\n\n", len(resp.OCIPathLayers)))
		for i, l := range resp.OCIPathLayers {
			b.WriteString(fmt.Sprintf("  %d. %s  %s  %s\n", i+1, l.Digest, formatutils.FormatSize(l.Size), l.Path))
		}
	}
	return b.String()
}
