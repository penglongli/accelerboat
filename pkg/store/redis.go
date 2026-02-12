/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package store

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
)

// LayerType defines the layer type
type LayerType string

const (
	DOCKERD    LayerType = "DOCKERD"
	CONTAINERD LayerType = "CONTAINERD"
	StaticFile LayerType = "STATIC"
)

// LayerLocatedInfo defines the layer located info
type LayerLocatedInfo struct {
	Layer   string    `json:"layer"`
	Type    LayerType `json:"type"`
	Located string    `json:"located"`
	Data    string    `json:"data"`
	TS      int64     `json:"ts"`
	Refer   int64     `json:"refer"`
}

// CacheStore defines the interface of cache store
type CacheStore interface {
	SaveOCILayer(ctx context.Context, ociType LayerType, layer, filePath string) error
	DeleteOCILayer(ctx context.Context, ociType LayerType, layer string) error
	SaveStaticLayer(ctx context.Context, layer, filePath string, printLog bool) error
	DeleteStaticLayer(ctx context.Context, layer string) error
	DeleteLocatedStaticLayer(ctx context.Context, located, layer string) error
	QueryLayers(ctx context.Context, layer string) ([]*LayerLocatedInfo, []*LayerLocatedInfo, error)

	CleanHostCache(ctx context.Context) error
}

// RedisStore defines the redis store object
type RedisStore struct {
	op          *options.AccelerBoatOption
	redisClient *redis.Client

	// used to do clean host cache
	localCache *sync.Map
}

var (
	globalRS *RedisStore
	syncOnce sync.Once
)

// GlobalRedisStore returns the global redis store instance
func GlobalRedisStore() CacheStore {
	syncOnce.Do(func() {
		op := options.GlobalOptions()
		redisClient := redis.NewClient(&redis.Options{
			Addr:     op.RedisAddress,
			Password: op.RedisPassword,
		})
		redisClient.AddHook(NewRedisHook())
		globalRS = &RedisStore{
			op:          op,
			redisClient: redisClient,
			localCache:  &sync.Map{},
		}
	})
	return globalRS
}

func (r *RedisStore) buildLayerKey(ociType LayerType) string {
	return fmt.Sprintf("%s/%s", r.op.Address, string(ociType))
}

func (r *RedisStore) buildLayerValue(filePath string) string {
	return fmt.Sprintf("%s:%d", filePath, time.Now().Unix())
}

func (r *RedisStore) parseLayerValue(value string) (string, int64, error) {
	vs := strings.Split(value, ":")
	if len(vs) != 2 {
		return "", 0, errors.Errorf("invalid layer value: %s", value)
	}
	tstr := vs[1]
	ts, err := strconv.ParseInt(tstr, 10, 64)
	if err != nil {
		return "", 0, errors.Wrapf(err, "invalid layer value: %s", tstr)
	}
	return vs[0], ts, nil
}

// SaveOCILayer save the dockerd/containerd layers with filepath
func (r *RedisStore) SaveOCILayer(ctx context.Context, ociType LayerType, layer, filePath string) error {
	key := r.buildLayerKey(ociType)
	value := r.buildLayerValue(filePath)
	if err := r.redisClient.HSet(ctx, layer, key, value).Err(); err != nil {
		return errors.Wrapf(err, "redis set key '%s' with vaule '%s' failed", key, filePath)
	}
	r.localCache.Store(layer, struct{}{})
	logger.V(3).InfoContextf(ctx, "cache save oci layer '%s = %s' success", key, filePath)
	return nil
}

// DeleteOCILayer delete the oci layers
func (r *RedisStore) DeleteOCILayer(ctx context.Context, ociType LayerType, layer string) error {
	key := r.buildLayerKey(ociType)
	if err := r.redisClient.HDel(ctx, layer, key).Err(); err != nil {
		return errors.Wrapf(err, "redis del key '%s' failed", key)
	}
	return nil
}

// SaveStaticLayer save static layer
func (r *RedisStore) SaveStaticLayer(ctx context.Context, layer string, filePath string, printLog bool) error {
	key := r.buildLayerKey(StaticFile)
	value := r.buildLayerValue(filePath)
	if err := r.redisClient.HSet(ctx, layer, key, value).Err(); err != nil {
		return errors.Wrapf(err, "redis set key '%s' with vaule '%s' failed", key, filePath)
	}
	if printLog {
		logger.InfoContextf(ctx, "cache save static layer '%s = %s' success", key, filePath)
	}
	r.localCache.Store(layer, struct{}{})
	return nil
}

// DeleteStaticLayer delete static layer
func (r *RedisStore) DeleteStaticLayer(ctx context.Context, layer string) error {
	key := r.buildLayerKey(StaticFile)
	if err := r.redisClient.HDel(ctx, layer, key).Err(); err != nil {
		return errors.Wrapf(err, "redis del key '%s' failed", key)
	}
	return nil
}

func (r *RedisStore) DeleteLocatedStaticLayer(ctx context.Context, located, layer string) error {
	key := fmt.Sprintf("%s/%s", located, string(StaticFile))
	if err := r.redisClient.HDel(ctx, layer, key).Err(); err != nil {
		return errors.Wrapf(err, "redis del key '%s' failed", key)
	}
	return nil
}

// CleanHostCache clean host cache
func (r *RedisStore) CleanHostCache(ctx context.Context) error {
	clean := func(wg *sync.WaitGroup, layer string) {
		defer wg.Done()
		keys := []string{
			r.buildLayerKey(StaticFile),
			r.buildLayerKey(CONTAINERD),
			r.buildLayerKey(DOCKERD),
		}
		for _, key := range keys {
			r.redisClient.HDel(ctx, layer, key)
		}
	}
	wg := &sync.WaitGroup{}
	counts := 0
	r.localCache.Range(func(key, value interface{}) bool {
		wg.Add(1)
		layer := key.(string)
		go clean(wg, layer)
		counts++
		return true
	})
	wg.Wait()
	logger.InfoContextf(ctx, "clean host cache %d success", counts)
	return nil
}

func (r *RedisStore) QueryLayers(ctx context.Context, layer string) ([]*LayerLocatedInfo, []*LayerLocatedInfo, error) {
	keyTypes := map[string]struct{}{
		string(StaticFile): {},
		string(CONTAINERD): {},
		string(DOCKERD):    {},
	}
	all, err := r.redisClient.HGetAll(ctx, layer).Result()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "redis get key '%s' failed", layer)
	}
	staticLayers := make([]*LayerLocatedInfo, 0)
	ociLayers := make([]*LayerLocatedInfo, 0)
	for k, v := range all {
		ks := strings.Split(k, "/")
		if len(ks) != 2 {
			continue
		}
		keyType := ks[1]
		if _, ok := keyTypes[keyType]; !ok {
			continue
		}

		filePath, ts, err := r.parseLayerValue(v)
		if err != nil {
			logger.ErrorContextf(ctx, "parse key '%s -> %s' layer value '%s' failed: %s",
				layer, k, v, err.Error())
			continue
		}
		if (time.Now().Unix() - ts) > 120 {
			continue
		}
		layerInfo := &LayerLocatedInfo{
			Layer:   layer,
			Type:    LayerType(keyType),
			Located: ks[0],
			Data:    filePath,
			TS:      ts,
		}
		switch keyType {
		case string(StaticFile):
			staticLayers = append(staticLayers, layerInfo)
		case string(CONTAINERD), string(DOCKERD):
			ociLayers = append(ociLayers, layerInfo)
		}
	}
	sort.Slice(staticLayers, func(i, j int) bool {
		return staticLayers[i].TS > staticLayers[j].TS
	})
	sort.Slice(ociLayers, func(i, j int) bool {
		return ociLayers[i].TS > ociLayers[j].TS
	})
	return getTopN(staticLayers, 50), getTopN(ociLayers, 50), nil
}

func getTopN(slice []*LayerLocatedInfo, n int) []*LayerLocatedInfo {
	if len(slice) <= n {
		return slice
	}
	return slice[:n]
}
