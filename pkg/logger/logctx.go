// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package logger

import (
	"context"
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

	contextMap, ok := ctx.Value(ContextKeyMessage).(map[string]string)
	if !ok {
		contextMap = make(map[string]string)
	}
	for k, v := range tags {
		contextMap[k] = v
	}
	return context.WithValue(ctx, ContextKeyMessage, contextMap)
}
