// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

import (
	"context"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"

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

func NewChangeWatcher(cfgPath string) (OptionChangeWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrapf(err, "create option watcher failed")
	}
	configFileDir := filepath.Dir(cfgPath)
	err = watcher.Add(configFileDir)
	if err != nil {
		return nil, errors.Wrapf(err, "add config filepath %s failed", cfgPath)
	}
	logger.Infof("config file path '%s' is added to watcher", cfgPath)
	return &optionChangeHandler{
		cfgPath: cfgPath,
		watcher: watcher,
	}, nil
}

type optionChangeHandler struct {
	cfgPath string
	watcher *fsnotify.Watcher
}

func (o *optionChangeHandler) Watch(ctx context.Context) <-chan *OptionChanges {
	ch := make(chan *OptionChanges)
	go func() {
		defer func() {
			o.watcher.Close()
			logger.Infof("option change watcher closed")
		}()
		for {
			select {
			case event, ok := <-o.watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write && event.Name == o.cfgPath {
					opc := o.handleFileChanged()
					if opc == nil {
						continue
					}
					logger.Infof("config file '%s' is modified", o.cfgPath)
					ch <- opc
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

func (o *optionChangeHandler) handleFileChanged() *OptionChanges {
	newOp, err := Parse(o.cfgPath)
	if err != nil {
		logger.Errorf("parse config file failed: %s", err.Error())
		return nil
	}
	changeOption(newOp)
	prevOp := &AccelerBoatOption{}
	currentOp := &AccelerBoatOption{}
	_ = utils.DeepCopyStruct(prev, prevOp)
	_ = utils.DeepCopyStruct(singleton, currentOp)
	return &OptionChanges{
		Prev:    prevOp,
		Current: currentOp,
	}
}
