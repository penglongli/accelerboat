// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ContextKey string

const (
	ContextKeyMessage ContextKey = "ACCELERBOAT_MESSAGE"
)

// WithContextFields Non-concurrency safe
func WithContextFields(ctx context.Context, fields ...string) context.Context {
	tagCapacity := len(fields) / 2
	tags := make(map[string]string)
	for i := 0; i < tagCapacity; i++ {
		key := fields[2*i]
		value := fields[2*i+1]
		tags[key] = value
	}

	zapFields := contextFields(ctx)
	for i := range zapFields {
		fd := zapFields[i]
		v, ok := tags[fd.Key]
		if !ok {
			continue
		}
		fd.String = v
		fd.Type = zapcore.StringType
		delete(tags, fd.Key)
	}
	for k, v := range tags {
		zapFields = append(zapFields, zapcore.Field{
			Key:    k,
			Type:   zapcore.StringType,
			String: v,
		})
	}

	return context.WithValue(ctx, ContextKeyMessage, zapFields)
}

func GetContextField(ctx context.Context, key string) string {
	val := ctx.Value(ContextKeyMessage)
	if val == nil {
		return ""
	}
	fields := val.([]zap.Field)
	for i := range fields {
		if fields[i].Key == key {
			return fields[i].String
		}
	}
	return ""
}

func contextFields(ctx context.Context) []zap.Field {
	if val := ctx.Value(ContextKeyMessage); val != nil {
		if fields, ok := val.([]zap.Field); ok {
			return fields
		}
	}
	return nil
}
