// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package lock

import (
	"context"
	"sync"

	"k8s.io/klog/v2"
)

// keyedMutex the local lock with string key
type keyedMutex struct {
	mutexes *sync.Map
}

// NewLocalLock create new localLock instance
func NewLocalLock() Interface {
	return &keyedMutex{
		mutexes: &sync.Map{},
	}
}

// Lock the key
func (m *keyedMutex) Lock(ctx context.Context, key string) {
	value, _ := m.mutexes.LoadOrStore(key, &sync.Mutex{})
	mtx := value.(*sync.Mutex)
	mtx.Lock()
}

// UnLock the key
func (m *keyedMutex) UnLock(ctx context.Context, key string) {
	value, _ := m.mutexes.Load(key)
	if value == nil {
		klog.Warningf("local unlock '%s' is empty", key)
		return
	}
	mtx := value.(*sync.Mutex)
	mtx.Unlock()
	return
}
