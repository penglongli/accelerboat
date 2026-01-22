// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package logger

import (
	"context"
)

type Verbose struct {
	level int
}

func V(level int) Verbose {
	return Verbose{
		level: level,
	}
}

func (v Verbose) Infof(format string, args ...interface{}) {
	if v.level > maxLevel {
		return
	}
	Infof(format, args...)
}

func (v Verbose) InfoContextf(ctx context.Context, format string, args ...interface{}) {
	if v.level > maxLevel {
		return
	}
	InfoContextf(ctx, format, args...)
}

func (v Verbose) Warnf(format string, args ...interface{}) {
	if v.level > maxLevel {
		return
	}
	Warnf(format, args...)
}

func (v Verbose) WarnContextf(ctx context.Context, format string, args ...interface{}) {
	if v.level > maxLevel {
		return
	}
	WarnContextf(ctx, format, args...)
}

func (v Verbose) Errorf(format string, args ...interface{}) {
	if v.level > maxLevel {
		return
	}
	Errorf(format, args...)
}

func (v Verbose) ErrorContextf(ctx context.Context, format string, args ...interface{}) {
	if v.level > maxLevel {
		return
	}
	ErrorContextf(ctx, format, args...)
}

func (v Verbose) Fatalf(format string, args ...interface{}) {
	if v.level > maxLevel {
		return
	}
	Fatalf(format, args...)
}

func (v Verbose) FatalContextf(ctx context.Context, format string, args ...interface{}) {
	if v.level > maxLevel {
		return
	}
	FatalContextf(ctx, format, args...)
}
