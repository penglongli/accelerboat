// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/server"
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
	op, err := options.Parse(*config, true)
	if err != nil {
		panic(errors.Wrapf(err, "parse options failed"))
	}
	opWatcher := options.NewChangeWatcher(*config)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		interrupt := make(chan os.Signal, 10)
		signal.Notify(interrupt, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM,
			syscall.SIGUSR1, syscall.SIGUSR2)
		for s := range interrupt {
			logger.Infof("Received signal %v from system. Exit!", s)
			cancel()
			return
		}
	}()
	svr := server.NewAccelerboatServer(ctx, op, opWatcher)
	if err = svr.Init(); err != nil {
		logger.Fatalf("server init failed: %v", err)
	}
	if err = svr.Run(); err != nil {
		logger.Fatalf("server exit: %s", err.Error())
	}
}
