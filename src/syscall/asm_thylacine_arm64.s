// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

//
// System call support for Thylacine, arm64.
//
// The kernel SVC ABI is Linux-arm64-shaped: number in R8, args in R0..R5,
// return value in R0. An error is a negative value in [-4095, -1] (the
// newer syscalls return a real -errno; the legacy file syscalls return a
// generic -1, which surfaces here as EPERM -- see #102). The errno-decode
// is the standard `CMN $4095, R0` / `BCC` pattern.
//
// These primitives do NOT call runtime.entersyscall: at Stage 3a the only
// callers are file ops whose blocking is bounded, and sysmon retakes the P
// for any pathological stall. The net layer (Stage 3c) adds the
// entersyscall-wrapped blocking path it needs.
//

// func Syscall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno)
TEXT ·Syscall(SB),NOSPLIT,$0-56
	MOVD	a1+8(FP), R0
	MOVD	a2+16(FP), R1
	MOVD	a3+24(FP), R2
	MOVD	ZR, R3
	MOVD	ZR, R4
	MOVD	ZR, R5
	MOVD	trap+0(FP), R8
	SVC
	CMN	$4095, R0
	BCC	oksc3
	MOVD	$-1, R1
	MOVD	R1, r1+32(FP)
	MOVD	ZR, r2+40(FP)
	NEG	R0, R0
	MOVD	R0, err+48(FP)
	RET
oksc3:
	MOVD	R0, r1+32(FP)
	MOVD	R1, r2+40(FP)
	MOVD	ZR, err+48(FP)
	RET

// func Syscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno)
TEXT ·Syscall6(SB),NOSPLIT,$0-80
	MOVD	a1+8(FP), R0
	MOVD	a2+16(FP), R1
	MOVD	a3+24(FP), R2
	MOVD	a4+32(FP), R3
	MOVD	a5+40(FP), R4
	MOVD	a6+48(FP), R5
	MOVD	trap+0(FP), R8
	SVC
	CMN	$4095, R0
	BCC	oksc6
	MOVD	$-1, R1
	MOVD	R1, r1+56(FP)
	MOVD	ZR, r2+64(FP)
	NEG	R0, R0
	MOVD	R0, err+72(FP)
	RET
oksc6:
	MOVD	R0, r1+56(FP)
	MOVD	R1, r2+64(FP)
	MOVD	ZR, err+72(FP)
	RET

// func RawSyscall(trap, a1, a2, a3 uintptr) (r1, r2, err uintptr)
TEXT ·RawSyscall(SB),NOSPLIT,$0-56
	MOVD	a1+8(FP), R0
	MOVD	a2+16(FP), R1
	MOVD	a3+24(FP), R2
	MOVD	ZR, R3
	MOVD	ZR, R4
	MOVD	ZR, R5
	MOVD	trap+0(FP), R8
	SVC
	CMN	$4095, R0
	BCC	okraw3
	MOVD	$-1, R1
	MOVD	R1, r1+32(FP)
	MOVD	ZR, r2+40(FP)
	NEG	R0, R0
	MOVD	R0, err+48(FP)
	RET
okraw3:
	MOVD	R0, r1+32(FP)
	MOVD	R1, r2+40(FP)
	MOVD	ZR, err+48(FP)
	RET

// func RawSyscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr)
TEXT ·RawSyscall6(SB),NOSPLIT,$0-80
	MOVD	a1+8(FP), R0
	MOVD	a2+16(FP), R1
	MOVD	a3+24(FP), R2
	MOVD	a4+32(FP), R3
	MOVD	a5+40(FP), R4
	MOVD	a6+48(FP), R5
	MOVD	trap+0(FP), R8
	SVC
	CMN	$4095, R0
	BCC	okraw6
	MOVD	$-1, R1
	MOVD	R1, r1+56(FP)
	MOVD	ZR, r2+64(FP)
	NEG	R0, R0
	MOVD	R0, err+72(FP)
	RET
okraw6:
	MOVD	R0, r1+56(FP)
	MOVD	R1, r2+64(FP)
	MOVD	ZR, err+72(FP)
	RET
