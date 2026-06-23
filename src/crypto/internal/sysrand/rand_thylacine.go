// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sysrand

import (
	"syscall"
	"unsafe"
)

// read fills b with cryptographically secure random bytes from the Thylacine
// kernel CSPRNG via SYS_GETRANDOM. The kernel's getrandom (flags=0) does not
// short-return once the pool is seeded, but the loop is kept for the partial-
// read contract every other GOOS upholds. A pre-seed call fails closed (the
// kernel refuses while unseeded) rather than blocking, surfacing as an error
// the caller (crypto/rand.Read) turns into a fatal -- matching getrandom on
// the unix ports.
func read(b []byte) error {
	for len(b) > 0 {
		n, _, errno := syscall.Syscall(syscall.SYS_GETRANDOM,
			uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0)
		if errno != 0 {
			return errno
		}
		b = b[n:]
	}
	return nil
}
