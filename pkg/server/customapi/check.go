// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/pkg/server/customapi/apitypes"
)

func (h *CustomHandler) CheckStaticLayer(c *gin.Context) (interface{}, error) {
	req := &apitypes.CheckStaticLayerRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, errors.Wrapf(err, "parse request failed")
	}
	fileSize, err := checkLocalLayer(req.LayerPath)
	if err != nil {
		return nil, errors.Wrapf(err, "check local layer failed")
	}
	if fileSize != req.ExpectedContentLength {
		return nil, fmt.Errorf("local file '%s' content-length '%d', not same as expcted '%d'",
			req.LayerPath, fileSize, req.ExpectedContentLength)
	}
}

func (h *CustomHandler) CheckOCILayer(c *gin.Context) (interface{}, error) {
	return nil, nil
}

func (h *CustomHandler) TransferLayerTCP(c *gin.Context) {

}

func checkLocalLayer(filePath string) (int64, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return 0, errors.Wrapf(err, "stat layer file '%s' failed", filePath)
	}
	return fi.Size(), nil
}
