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

// RedisHook 实现 go-redis 的 Hook 接口
type RedisHook struct{}

func NewRedisHook() *RedisHook {
	return &RedisHook{}
}

// 定义上下文key（用于存储操作开始时间）
type ctxKey int

const (
	redisStartTimestampKey ctxKey = iota
)

// BeforeProcess 记录操作开始时间
func (h *RedisHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	// 将开始时间存入上下文
	ctx = context.WithValue(ctx, redisStartTimestampKey, time.Now())
	return ctx, nil
}

// AfterProcess 计算耗时、记录错误、更新metrics
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

// BeforeProcessPipeline 批量操作前记录开始时间
func (h *RedisHook) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	ctx = context.WithValue(ctx, redisStartTimestampKey, time.Now())
	return ctx, nil
}

// AfterProcessPipeline 批量操作后处理
func (h *RedisHook) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	startTime, ok := ctx.Value(redisStartTimestampKey).(time.Time)
	if !ok {
		startTime = time.Now()
	}

	for _, cmd := range cmds {
		// 为每个批量命令单独计算耗时（或复用批量开始时间，根据需求调整）
		cmdCtx := context.WithValue(ctx, redisStartTimestampKey, startTime)
		_ = h.AfterProcess(cmdCtx, cmd)
	}
	return nil
}

// sanitizeArgs 脱敏处理参数
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
