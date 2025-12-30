// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package utils

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
)

// ToJson marshal object to json
func ToJson(obj interface{}) []byte {
	bs, _ := json.Marshal(obj) // nolint
	return bs
}

// DeepCopyStruct deep copy object
func DeepCopyStruct(src, dest interface{}) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(src); err != nil {
		return err
	}
	return gob.NewDecoder(&buf).Decode(dest)
}

// StringASCII return the acii of string
func StringASCII(str string) int64 {
	var result int64
	for i := range str {
		// take the index to do weighting
		result += int64(i) + int64(str[i])
	}
	return result
}
