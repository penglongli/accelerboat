// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package logger

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Option struct {
	Filename   string
	MaxSize    int
	MaxAge     int
	MaxBackups int
}

var (
	zapLogger *zap.Logger
)

func InitLogger(op *Option) {
	lumberjackLogger := &lumberjack.Logger{
		Filename:   op.Filename,
		MaxSize:    op.MaxSize,
		MaxAge:     op.MaxAge,
		MaxBackups: op.MaxBackups,
		Compress:   true,
	}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			TimeKey:      "time",
			LevelKey:     "level",
			MessageKey:   "msg",
			CallerKey:    "C",
			EncodeTime:   zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000"),
			EncodeLevel:  zapcore.CapitalLevelEncoder,
			EncodeCaller: zapcore.ShortCallerEncoder,
		}),
		zapcore.AddSync(lumberjackLogger),
		zap.InfoLevel,
	)
	zapLogger = zap.New(core, zap.AddCaller())
}

func Infof(format string, args ...interface{}) {
	zapLogger.Info(format, args)
}

func InfoContextf(ctx context.Context, format string, args ...interface{}) {

}

func WarnContextf(ctx context.Context, format string, args ...interface{}) {
	glog.WarningDepth(1, fmt.Sprintf(format+getKeyValueFromCtx(ctx), args...))
}

func ErrorContextf(ctx context.Context, format string, args ...interface{}) {
	glog.ErrorDepth(1, fmt.Sprintf(format+getKeyValueFromCtx(ctx), args...))
}

func FatalContextf(ctx context.Context, format string, args ...interface{}) {
	glog.FatalDepth(1, fmt.Sprintf(format+getKeyValueFromCtx(ctx), args...))
}
