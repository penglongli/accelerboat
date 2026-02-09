// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/penglongli/accelerboat/pkg/recorder"
)

func (h *CustomHandler) Recorder(c *gin.Context) (interface{}, error) {
	limit := 100
	if s := c.Query("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	events := recorder.Global.List(limit)
	if events == nil {
		return gin.H{"events": []interface{}{}}, nil
	}
	out := make([]interface{}, 0, len(events))
	for _, e := range events {
		out = append(out, map[string]interface{}{
			"type":      string(e.Type),
			"timestamp": e.Timestamp,
			"details":   e.Details,
		})
	}
	return gin.H{"events": out}, nil
}

func (h *CustomHandler) TorrentStatus(c *gin.Context) (interface{}, error) {
	return nil, nil
}
