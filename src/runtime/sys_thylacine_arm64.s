// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//
// System calls and other sys.stuff for arm64, Thylacine.
//
// Thylacine's SVC ABI is Linux-arm64-shaped: syscall number in R8 (x8),
// arguments in R0..R5 (x0..x5), return value in R0 (x0). Errno is returned
// as a negative value in [-4095, -2]. Clock reads use the AT_VDSO_CLOCK page
// fast-path (read_cntvct below + the page; see os_thylacine.go nanotime1/
// walltime) with a clock_gettime syscall fallback.
//
// Three mechanisms diverge from the Linux base and are handled in
// os_thylacine.go / mem_thylacine.go, not here:
//   - thread_spawn is entry-point (not clone-resume) -- see thread_entry.
//   - futex is split into torpor_wait / torpor_wake.
//   - mmap is BURROW_ATTACH (eager, kernel-chosen address, no MAP_FIXED).
//

#include "go_asm.h"
#include "go_tls.h"
#include "textflag.h"

#define SYS_read		9
#define SYS_write		10
#define SYS_close		11
#define SYS_getrandom		20
#define SYS_set_tid_address	36
#define SYS_burrow_detach	38
#define SYS_burrow_attach_lazy	83
#define SYS_burrow_decommit	84
#define SYS_torpor_wait		39
#define SYS_torpor_wake		40
#define SYS_thread_spawn	41
#define SYS_thread_exit		42
#define SYS_exit_group		60
#define SYS_getpid		72
#define SYS_clock_gettime	75
#define SYS_yield		87

// func exit(code int32)
TEXT runtime·exit(SB),NOSPLIT|NOFRAME,$0-4
	MOVW	code+0(FP), R0
	MOVD	$SYS_exit_group, R8
	SVC
	RET

// func exitThread(wait *atomic.Uint32)
// Clear *wait so the OS thread's stack can be reused, then exit this one
// thread. No stack access between the store and the SVC: another thread may
// free the stack the instant *wait is cleared.
TEXT runtime·exitThread(SB),NOSPLIT|NOFRAME,$0-8
	MOVD	wait+0(FP), R0
	MOVW	$0, R1
	STLRW	R1, (R0)
	MOVD	$SYS_thread_exit, R8
	SVC
	JMP	0(PC)

// func write1(fd uintptr, p unsafe.Pointer, n int32) int32
TEXT runtime·write1(SB),NOSPLIT|NOFRAME,$0-28
	MOVD	fd+0(FP), R0
	MOVD	p+8(FP), R1
	MOVW	n+16(FP), R2
	MOVD	$SYS_write, R8
	SVC
	MOVW	R0, ret+24(FP)
	RET

// func read(fd int32, p unsafe.Pointer, n int32) int32
TEXT runtime·read(SB),NOSPLIT|NOFRAME,$0-28
	MOVW	fd+0(FP), R0
	MOVD	p+8(FP), R1
	MOVW	n+16(FP), R2
	MOVD	$SYS_read, R8
	SVC
	MOVW	R0, ret+24(FP)
	RET

// func closefd(fd int32) int32
TEXT runtime·closefd(SB),NOSPLIT|NOFRAME,$0-12
	MOVW	fd+0(FP), R0
	MOVD	$SYS_close, R8
	SVC
	MOVW	R0, ret+8(FP)
	RET

// func getpid() int
TEXT runtime·getpid(SB),NOSPLIT|NOFRAME,$0-8
	MOVD	$SYS_getpid, R8
	SVC
	MOVD	R0, ret+0(FP)
	RET

// func openReadRoot(path unsafe.Pointer, n int32) int32
// SYS_OPEN(SYS_WALK_OPEN_FROM_ROOT, path, n, OREAD): open an existing path
// for reading, resolved from the Territory root. Returns the fd, or a
// negative value on error. Used by getCPUCount to read /ctl/sched at osinit.
TEXT runtime·openReadRoot(SB),NOSPLIT|NOFRAME,$0-20
	MOVD	$-1, R0			// SYS_WALK_OPEN_FROM_ROOT
	MOVD	path+0(FP), R1
	MOVW	n+8(FP), R2
	MOVD	$0, R3			// OREAD
	MOVD	$65, R8			// SYS_OPEN
	SVC
	MOVW	R0, ret+16(FP)
	RET

// readdirRaw(fd, buf, n) -> SYS_READDIR (56). Reads the next run of 9P2000.L
// dirents into buf; returns the byte count, or 0 at end-of-directory. The fd's
// offset (the resume cookie) advances in the kernel across calls, so repeated
// calls drain the directory. Used by goenvs to enumerate /env (a directory; a
// plain read on it returns -1, so readdir is the enumeration path).
TEXT runtime·readdirRaw(SB),NOSPLIT|NOFRAME,$0-28
	MOVW	fd+0(FP), R0
	MOVD	p+8(FP), R1
	MOVW	n+16(FP), R2
	MOVD	$56, R8			// SYS_READDIR
	SVC
	MOVW	R0, ret+24(FP)
	RET

// func clock_gettime(clockid int32, ts *timespec)
// The timespec is allocated by the Go caller (walltime / nanotime1); this is
// a pure register shuffle, so there is no stack-frame layout hazard here.
TEXT runtime·clock_gettime(SB),NOSPLIT|NOFRAME,$0-16
	MOVW	clockid+0(FP), R0
	MOVD	ts+8(FP), R1
	MOVD	$SYS_clock_gettime, R8
	SVC
	RET

// func read_cntvct() uint64
// The architectural virtual counter, EL0-enabled by the kernel (CNTKCTL_EL1.
// EL0VCTEN). The vDSO clock fast-path (os_thylacine.go) reads this + the kernel
// timekeeping page to compute the clock with NO syscall. No ISB: CNTVCT is
// architecturally monotonic, so a few-cycle speculative skew stays monotonic
// (matches the FreeBSD getCntxct precedent).
TEXT runtime·read_cntvct(SB),NOSPLIT|NOFRAME,$0-8
	MRS	CNTVCT_EL0, R0
	MOVD	R0, ret+0(FP)
	RET

// func torpor_wait(addr unsafe.Pointer, val uint32, us int64) int32
// Atomically: if *addr == val, sleep up to us microseconds (us == 0 means
// forever, per the kernel). If *addr != val, return immediately.
TEXT runtime·torpor_wait(SB),NOSPLIT|NOFRAME,$0-28
	MOVD	addr+0(FP), R0
	MOVWU	val+8(FP), R1
	MOVD	us+16(FP), R2
	MOVD	$SYS_torpor_wait, R8
	SVC
	MOVW	R0, ret+24(FP)
	RET

// func torpor_wake(addr unsafe.Pointer, cnt uint32) int32
TEXT runtime·torpor_wake(SB),NOSPLIT|NOFRAME,$0-20
	MOVD	addr+0(FP), R0
	MOVWU	cnt+8(FP), R1
	MOVD	$SYS_torpor_wake, R8
	SVC
	MOVW	R0, ret+16(FP)
	RET

// func osyield()
// #33: a REAL yield. SYS_YIELD requeues this M behind any runnable peer
// queued on its CPU and dispatches the peer; with no local competition it
// returns immediately (the kernel-side fast path). Replaces the pre-#33
// torpor_wait(&sleepDummy, 1, 1) mismatch-return, which was a scheduling
// point in name only -- it never actually gave up the CPU, degrading the
// spinbit-mutex passive tier and every runtime spin loop (36.8M calls per
// go build). The linux SYS_sched_yield shape.
TEXT runtime·osyield(SB),NOSPLIT|NOFRAME,$0
	MOVD	$SYS_yield, R8
	SVC
	RET

// func getrandom(p unsafe.Pointer, n uintptr, flags uint32) int32
TEXT runtime·getrandom(SB),NOSPLIT|NOFRAME,$0-28
	MOVD	p+0(FP), R0
	MOVD	n+8(FP), R1
	MOVWU	flags+16(FP), R2
	MOVD	$SYS_getrandom, R8
	SVC
	MOVW	R0, ret+24(FP)
	RET

// func sysBurrowAttachLazy(n uintptr) uintptr
// Reserve [ret, ret+n) as anonymous, demand-zero, RW memory: no physical
// pages until first touch (the Linux overcommit contract). Returns the
// kernel-chosen base VA on success (a small positive VA), or a negative errno.
TEXT runtime·sysBurrowAttachLazy(SB),NOSPLIT|NOFRAME,$0-16
	MOVD	n+0(FP), R0
	MOVD	$SYS_burrow_attach_lazy, R8
	SVC
	MOVD	R0, ret+8(FP)
	RET

// func sysBurrowDetach(v unsafe.Pointer, n uintptr) int32
TEXT runtime·sysBurrowDetach(SB),NOSPLIT|NOFRAME,$0-20
	MOVD	v+0(FP), R0
	MOVD	n+8(FP), R1
	MOVD	$SYS_burrow_detach, R8
	SVC
	MOVW	R0, ret+16(FP)
	RET

// func sysBurrowDecommit(v unsafe.Pointer, n uintptr) int32
// Drop the resident pages of [v, v+n) (a madvise(DONTNEED) analog): clear the
// PTEs and free the pages; the reservation stays and a later touch re-faults a
// fresh zero page. Returns 0 on success, or a negative errno.
TEXT runtime·sysBurrowDecommit(SB),NOSPLIT|NOFRAME,$0-20
	MOVD	v+0(FP), R0
	MOVD	n+8(FP), R1
	MOVD	$SYS_burrow_decommit, R8
	SVC
	MOVW	R0, ret+16(FP)
	RET

// func thread_spawn(entry, sp uintptr, arg unsafe.Pointer, tls, ptid uintptr) int32
TEXT runtime·thread_spawn(SB),NOSPLIT|NOFRAME,$0-44
	MOVD	entry+0(FP), R0
	MOVD	sp+8(FP), R1
	MOVD	arg+16(FP), R2
	MOVD	tls+24(FP), R3
	MOVD	ptid+32(FP), R4
	MOVD	$SYS_thread_spawn, R8
	SVC
	MOVW	R0, ret+40(FP)
	RET

// thread_entry is the raw entry point for an OS thread created by
// thread_spawn. The kernel starts execution here with x0 = arg (the *m we
// passed), SP = mp.g0.stack.hi, TPIDR_EL0 = 0, every other register zeroed,
// and IRQs enabled. We set g = mp.g0, g.m = mp, and enter mstart -- the same
// hand-off the Linux clone child performs inline after the clone returns.
TEXT runtime·thread_entry(SB),NOSPLIT|NOFRAME,$0
	MOVD	R0, R1			// R1 = mp
	MOVD	m_g0(R1), g		// g = mp.g0   (g is R28)
	MOVD	R1, g_m(g)		// g.m = mp
	BL	runtime·mstart(SB)
	// mstart must not return; if it does, exit this thread.
	MOVD	$SYS_thread_exit, R8
	SVC
	JMP	0(PC)
