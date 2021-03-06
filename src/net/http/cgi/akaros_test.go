// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build akaros

package cgi

import (
	"os"
	"strconv"
	"testing"
)

func isProcessRunning(t *testing.T, pid int) bool {
	_, err := os.Stat("/9/proc/" + strconv.Itoa(pid))
	return err == nil
}
