// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package lock

import (
	"context"
)

// Interface defines the discovery interface
type Interface interface {
	Lock(ctx context.Context, key string)
	UnLock(ctx context.Context, key string)
}
