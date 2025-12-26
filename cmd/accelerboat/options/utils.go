// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package options

// StringASCII return the acii of string
func StringASCII(str string) int64 {
	var result int64
	for i := range str {
		// take the index to do weighting
		result += int64(i) + int64(str[i])
	}
	return result
}
