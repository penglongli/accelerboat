// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"flag"
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

}
