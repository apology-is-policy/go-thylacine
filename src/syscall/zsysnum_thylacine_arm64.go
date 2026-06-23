// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine && arm64

package syscall

// Thylacine syscall numbers, from kernel/include/thylacine/syscall.h.
// The kernel SVC ABI is Linux-arm64-shaped (number in R8, args R0..R5,
// return in R0), but the number space is Thylacine's own.
const (
	SYS_EXITS              = 0
	SYS_PUTS               = 1
	SYS_PIPE               = 8
	SYS_READ               = 9
	SYS_WRITE              = 10
	SYS_CLOSE              = 11
	SYS_DUP                = 12
	SYS_GETRANDOM          = 20
	SYS_WAIT_PID           = 22
	SYS_POLL               = 29
	SYS_WALK_OPEN          = 34
	SYS_CHROOT             = 35
	SYS_SET_TID_ADDRESS    = 36
	SYS_BURROW_ATTACH      = 37
	SYS_BURROW_DETACH      = 38
	SYS_TORPOR_WAIT        = 39
	SYS_TORPOR_WAKE        = 40
	SYS_THREAD_SPAWN       = 41
	SYS_THREAD_EXIT        = 42
	SYS_POSTNOTE           = 47
	SYS_NOTE_MASK          = 48
	SYS_SPAWN_FULL_ARGV    = 49
	SYS_FSTAT              = 50
	SYS_LSEEK              = 51
	SYS_WALK_CREATE        = 54
	SYS_FSYNC              = 55
	SYS_READDIR            = 56
	SYS_RENAME             = 57
	SYS_UNLINK             = 58
	SYS_WSTAT              = 59
	SYS_EXIT_GROUP         = 60
	SYS_OPEN               = 65
	SYS_CHDIR              = 69
	SYS_GETCWD             = 70
	SYS_FD2PATH            = 71
	SYS_GETPID             = 72
	SYS_GETUID             = 73
	SYS_GETGID             = 74
	SYS_CLOCK_GETTIME      = 75
	SYS_BURROW_ATTACH_LAZY = 83
	SYS_BURROW_DECOMMIT    = 84
)

// SYS_WALK_OPEN_FROM_ROOT is the start_fd sentinel that resolves a path
// against the caller's Territory root (kernel/include/thylacine/syscall.h).
const walkFromRoot = ^uintptr(0) // (u64)-1
