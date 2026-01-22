// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"context"
	"os"
	"time"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/utils"
)

type OptionChanges struct {
	Prev    *AccelerBoatOption `json:"prev"`
	Current *AccelerBoatOption `json:"current"`
}

type OptionChangeWatcher interface {
	Watch(ctx context.Context) <-chan *OptionChanges
}

func NewChangeWatcher(cfgPath string) OptionChangeWatcher {
	return &optionChangeHandler{
		cfgPath: cfgPath,
	}
}

type optionChangeHandler struct {
	cfgPath string
}

func (o *optionChangeHandler) Watch(ctx context.Context) <-chan *OptionChanges {
	ch := make(chan *OptionChanges)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer func() {
			ticker.Stop()
			logger.Infof("option change watcher closed")
		}()
		bs, _ := os.ReadFile(o.cfgPath) // nolint
		prevContent := utils.BytesToString(bs)
		for {
			select {
			case <-ticker.C:
				bs, _ = os.ReadFile(o.cfgPath)
				currentContent := utils.BytesToString(bs)
				if prevContent == currentContent {
					continue
				}
				prevContent = currentContent
				opc := o.handleFileChanged()
				if opc == nil {
					continue
				}
				logger.Infof("config file '%s' is modified", o.cfgPath)
				ch <- opc

			case <-ctx.Done():
				close(ch)
				return
			}
		}
	}()
	return ch
}

func (o *optionChangeHandler) handleFileChanged() *OptionChanges {
	if _, err := Parse(o.cfgPath, false); err != nil {
		logger.Errorf("parse config file failed: %s", err.Error())
		return nil
	}
	prevOp := &AccelerBoatOption{}
	currentOp := &AccelerBoatOption{}
	_ = utils.DeepCopyStruct(prev, prevOp)
	_ = utils.DeepCopyStruct(singleton, currentOp)
	return &OptionChanges{
		Prev:    prevOp,
		Current: currentOp,
	}
}
