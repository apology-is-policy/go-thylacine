// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// Errno values the runtime cares about (Thylacine errnos are POSIX-aligned;
// see kernel/include/thylacine/errno.h).
const (
	_EINTR  = 4
	_EAGAIN = 11
	_ENOMEM = 12
)

// clockid arguments to SYS_CLOCK_GETTIME (matches Linux clockid_t and the
// kernel's T_CLOCK_* constants).
const (
	_CLOCK_REALTIME  = 0
	_CLOCK_MONOTONIC = 1
)

// timespec matches the kernel's t_timespec: { i64 tv_sec; i64 tv_nsec; }.
type timespec struct {
	tv_sec  int64
	tv_nsec int64
}

//go:nosplit
func (ts *timespec) setNsec(ns int64) {
	ts.tv_sec = ns / 1e9
	ts.tv_nsec = ns % 1e9
}
