// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package logger

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Option struct {
	Filename   string
	MaxSize    int
	MaxAge     int
	MaxBackups int
	Level      int
}

var (
	zapLogger *zap.Logger
	maxLevel  int
)

func InitLogger(op *Option) {
	if op.Level <= 0 {
		maxLevel = 2
	} else {
		maxLevel = op.Level
	}
	lumberjackLogger := &lumberjack.Logger{
		Filename:   op.Filename,
		MaxSize:    op.MaxSize,
		MaxAge:     op.MaxAge,
		MaxBackups: op.MaxBackups,
		Compress:   true,
	}
	syncer := zapcore.NewMultiWriteSyncer(zapcore.AddSync(lumberjackLogger), zapcore.AddSync(os.Stdout))
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
		syncer,
		zap.InfoLevel,
	)
	zapLogger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
}

func Infof(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	zapLogger.Info(msg)
}

func InfoContextf(ctx context.Context, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fields := contextFields(ctx)
	zapLogger.Info(msg, fields...)
}

func Warnf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	zapLogger.Warn(msg)
}

func WarnContextf(ctx context.Context, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fields := contextFields(ctx)
	zapLogger.Warn(msg, fields...)
}

func Errorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	zapLogger.Error(msg)
}

func ErrorContextf(ctx context.Context, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fields := contextFields(ctx)
	zapLogger.Error(msg, fields...)
}

func Fatalf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	zapLogger.Fatal(msg)
}

func FatalContextf(ctx context.Context, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fields := contextFields(ctx)
	zapLogger.Fatal(msg, fields...)
}
