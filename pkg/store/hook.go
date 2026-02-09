// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/metrics"
)

// RedisHook implements go-redis Hook interface for metrics and logging.
type RedisHook struct{}

// NewRedisHook returns a new RedisHook.
func NewRedisHook() *RedisHook {
	return &RedisHook{}
}

type ctxKey int

const (
	redisStartTimestampKey ctxKey = iota
)

// BeforeProcess stores the operation start time in context.
func (h *RedisHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	// Store start time in context
	ctx = context.WithValue(ctx, redisStartTimestampKey, time.Now())
	return ctx, nil
}

// AfterProcess records duration, errors, and updates metrics.
func (h *RedisHook) AfterProcess(ctx context.Context, cmd redis.Cmder) error {
	startTime, ok := ctx.Value(redisStartTimestampKey).(time.Time)
	if !ok {
		startTime = time.Now()
	}
	duration := time.Since(startTime)
	durationMillis := duration.Milliseconds()

	cmdName := strings.ToUpper(cmd.Name())
	cmdArgs := sanitizeArgs(cmd.Args())
	status := "success"
	err := cmd.Err()
	if err != nil && !errors.Is(err, redis.Nil) {
		logger.WarnContextf(ctx, "Redis operation failed | command: %s | args: %v | duration: %dms | error: %v",
			cmdName, cmdArgs, durationMillis, err,
		)
		metrics.RecordError(metrics.ComponentRedis, cmdName)
		status = "error"
	}
	metrics.RedisOperationsTotal.WithLabelValues(cmdName, status).Inc()
	metrics.RedisOperationDuration.WithLabelValues(cmdName, status).Observe(float64(durationMillis))
	return nil
}

// BeforeProcessPipeline stores the pipeline start time in context.
func (h *RedisHook) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	ctx = context.WithValue(ctx, redisStartTimestampKey, time.Now())
	return ctx, nil
}

// AfterProcessPipeline processes each command in the pipeline for metrics (reuses pipeline start time).
func (h *RedisHook) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	startTime, ok := ctx.Value(redisStartTimestampKey).(time.Time)
	if !ok {
		startTime = time.Now()
	}

	for _, cmd := range cmds {
		// Use pipeline start time for each command's duration (or use per-cmd timing if needed)
		cmdCtx := context.WithValue(ctx, redisStartTimestampKey, startTime)
		_ = h.AfterProcess(cmdCtx, cmd)
	}
	return nil
}

func sanitizeArgs(args []interface{}) []interface{} {
	sanitized := make([]interface{}, len(args))
	for i, arg := range args {
		if s, ok := arg.(string); ok {
			if strings.Contains(strings.ToLower(s), "password") ||
				strings.Contains(strings.ToLower(s), "token") {
				sanitized[i] = "***"
				continue
			}
		}
		sanitized[i] = arg
	}
	return sanitized
}
