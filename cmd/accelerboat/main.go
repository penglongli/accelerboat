// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"

	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
)

var (
	config = flag.String("f", "", "config file path")
)

func init() {
	flag.Parse()
	if config == nil || *config == "" {
		panic("config file is required")
	}
}

func main() {
	op, err := options.Parse(*config)
	if err != nil {
		panic(errors.Wrapf(err, "parse options failed"))
	}
	opWatcher, err := options.NewChangeWatcher(*config)
	if err != nil {
		panic(errors.Wrapf(err, "create options watcher failed"))
	}

}
