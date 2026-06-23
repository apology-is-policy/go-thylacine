// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package os

import "syscall"

// executable returns the path of the running program. Thylacine exposes no
// self-image path at v1.0 (no /proc/<pid>/text); callers fall back to
// os.Args[0].
func executable() (string, error) {
	return "", syscall.ENOSYS
}
