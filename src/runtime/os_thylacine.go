// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"internal/goarch"
	"internal/runtime/atomic"
	"unsafe"
)

// Thylacine is a non-unix GOOS (like plan9): it has no POSIX signals, so
// signal_unix.go is excluded and the signal surface below is stubbed. Unlike
// plan9, it has a futex-equivalent (torpor), so lock_futex.go is reused and
// the parking primitives are futexsleep / futexwakeup.

// mOS.waitsema backs the futex-based semaphore in lock_futex.go (semasleep /
// semawakeup); it is the only per-m OS state Thylacine needs.
type mOS struct {
	waitsema uint32
}

// sigset is empty: Thylacine has no signal masks. Note delivery (the eventual
// signal analog) lands at Stage 2.
type sigset struct{}

// gsignalStack is unused on Thylacine (no alternate signal stack).
type gsignalStack struct{}

// _NSIG bounds the signal-number bitmaps in sigqueue.go. Thylacine delivers no
// signals yet; the Linux-shaped range is kept for the eventual note mapping.
const _NSIG = 65

// --- raw syscall stubs (sys_thylacine_arm64.s) ---

func exit(code int32)
func exitThread(wait *atomic.Uint32)

//go:noescape
func write1(fd uintptr, p unsafe.Pointer, n int32) int32

//go:noescape
func read(fd int32, p unsafe.Pointer, n int32) int32

func closefd(fd int32) int32

func getpid() int

//go:noescape
func clock_gettime(clockid int32, ts *timespec)

//go:noescape
func torpor_wait(addr unsafe.Pointer, val uint32, us int64) int32

//go:noescape
func torpor_wake(addr unsafe.Pointer, cnt uint32) int32

//go:noescape
func getrandom(p unsafe.Pointer, n uintptr, flags uint32) int32

//go:noescape
func thread_spawn(entry, sp uintptr, arg unsafe.Pointer, tls, ptid uintptr) int32

func thread_entry()

// --- futex on torpor (the second hard divergence) ---
//
// torpor_wait(addr, expected, timeout_us): if *addr == expected, register and
// sleep. The kernel's timeout_us convention (kernel/torpor.c
// sys_torpor_wait_for_proc) is:
//     < 0  : block indefinitely (no deadline)  -- Go's "sleep forever".
//     == 0 : probe; register, recheck, return at ONCE (no real sleep).
//     > 0  : block at most timeout_us microseconds.
// So "forever" is a NEGATIVE timeout, NOT 0. A prior version passed 0 for the
// ns<0 case believing 0 meant forever; the kernel read it as "return at once",
// so every idle M re-parked in a tight loop -- ~5M futex syscalls/sec, all cores
// pinned, the go-build wall time. (#343: a `go tool compile` of a 2-line file
// burned ~300s in 982M torpor_wait calls; the fix is the single -1 below.)
// For a BOUNDED Go sleep a computed-zero microsecond count is clamped up to 1 so
// it stays a real >0 sleep rather than the 0-microsecond immediate-return probe.

//go:nosplit
func futexsleep(addr *uint32, val uint32, ns int64) {
	if ns < 0 {
		torpor_wait(unsafe.Pointer(addr), val, -1) // forever (kernel: <0 = indefinite)
		return
	}
	us := ns / 1000
	if us == 0 {
		us = 1 // a 0-us count is the kernel's immediate-return probe, not a sleep
	}
	torpor_wait(unsafe.Pointer(addr), val, us)
}

//go:nosplit
func futexwakeup(addr *uint32, cnt uint32) {
	torpor_wake(unsafe.Pointer(addr), cnt)
}

// sleepDummy backs usleep. It is never written or woken, so a matching-value
// wait always times out.
var sleepDummy uint32

// osyield issues SYS_YIELD (#33) -- a real voluntary yield, implemented in
// sys_thylacine_arm64.s. The kernel requeues this M behind any runnable peer
// queued on its CPU (and runs the peer), or returns immediately when there
// is no local competition. Pre-#33 this was a torpor_wait mismatch-return
// that never actually yielded the CPU.
func osyield()

//go:nosplit
func osyield_no_g() {
	osyield()
}

//go:nosplit
func usleep(us uint32) {
	u := int64(us)
	if u == 0 {
		u = 1 // a 0-us count is the kernel's immediate-return probe, not a sleep
	}
	// *sleepDummy == 0 == 0, so this parks for up to u microseconds (no waker).
	torpor_wait(unsafe.Pointer(&sleepDummy), 0, u)
}

//go:nosplit
func usleep_no_g(us uint32) {
	usleep(us)
}

// --- time ---
//
// The Thylacine kernel maps a read-only timekeeping page (the vDSO) into every
// Proc and delivers its address in the AT_VDSO_CLOCK auxv entry (captured in
// sysargs below). Reading CNTVCT_EL0 + that page computes CLOCK_MONOTONIC /
// CLOCK_REALTIME with NO syscall -- the lever that kills the scheduler's ~740M
// SYS_CLOCK_GETTIME nanotime() churn (Thylacine #343). When the page is absent
// (an older kernel, or an OOM at exec), every read falls back to the syscall.
// See docs/VDSO-DESIGN.md in the Thylacine tree.

const (
	_AT_NULL       = 0
	_AT_VDSO_CLOCK = 0x5654
	vdsoClockMagic = 0x5644534f4c4b3031 // "VDSOLK01"
	vdsoClockVers  = 1
	nsPerSec       = 1000000000
)

// vdsoClock mirrors struct vdso_clock (kernel/include/thylacine/vdso.h). The
// reader treats it as read-only; the kernel updates wallOffsetNs with a single
// aligned-u64 atomic store on SYS_CLOCK_SETTIME (old-or-new, never torn -- no
// seqlock), so an atomic.Load64 of that field suffices.
type vdsoClock struct {
	magic        uint64
	version      uint64
	freq         uint64
	wallOffsetNs uint64
	_            [4]uint64
}

// vdsoClockBase is the validated page pointer, or nil to fall back to the
// syscall. Set once in sysargs at startup; read-only thereafter.
var vdsoClockBase *vdsoClock

// read_cntvct returns the architectural virtual counter (CNTVCT_EL0), EL0-
// enabled by the kernel. sys_thylacine_arm64.s.
//
//go:noescape
func read_cntvct() uint64

// monoFromCnt replicates the kernel's timer_now_ns() split form exactly (so the
// vDSO value is bit-identical to what SYS_CLOCK_GETTIME would return): the
// quotient/remainder split avoids the cnt*1e9 u64 overflow.
//
//go:nosplit
func monoFromCnt(cnt, freq uint64) uint64 {
	return (cnt/freq)*nsPerSec + (cnt%freq)*nsPerSec/freq
}

//go:nosplit
func nanotime1() int64 {
	if pg := vdsoClockBase; pg != nil {
		return int64(monoFromCnt(read_cntvct(), pg.freq))
	}
	var ts timespec
	clock_gettime(_CLOCK_MONOTONIC, &ts)
	return ts.tv_sec*1e9 + ts.tv_nsec
}

//go:nosplit
func walltime() (sec int64, nsec int32) {
	if pg := vdsoClockBase; pg != nil {
		real := monoFromCnt(read_cntvct(), pg.freq) + atomic.Load64(&pg.wallOffsetNs)
		return int64(real / nsPerSec), int32(real % nsPerSec)
	}
	var ts timespec
	clock_gettime(_CLOCK_REALTIME, &ts)
	return ts.tv_sec, int32(ts.tv_nsec)
}

// sysargs walks the auxv (after argv + envp on the initial stack) for
// AT_VDSO_CLOCK and, if the page validates, caches its pointer. Thylacine has no
// other auxv consumer (startup entropy comes from getrandom, not AT_RANDOM), so
// this is the whole auxv-parse path; auxv_none.go's no-op sysargs is excluded
// for thylacine.
func sysargs(argc int32, argv **byte) {
	n := argc + 1
	// skip over argv to the envp NULL terminator
	for argv_index(argv, n) != nil {
		n++
	}
	n++ // skip the NULL separator; argv+n is now the auxv
	auxvp := (*[1 << 28]uintptr)(add(unsafe.Pointer(argv), uintptr(n)*goarch.PtrSize))
	for i := 0; auxvp[i] != _AT_NULL; i += 2 {
		if auxvp[i] == _AT_VDSO_CLOCK {
			pg := (*vdsoClock)(unsafe.Pointer(auxvp[i+1]))
			if pg.magic == vdsoClockMagic && pg.version == vdsoClockVers && pg.freq != 0 {
				vdsoClockBase = pg
			}
			return
		}
	}
}

// --- init ---

func osinit() {
	physPageSize = 4096 // Thylacine arm64: PAGE_SHIFT == 12
	numCPUStartup = getCPUCount()
}

//go:noescape
func openReadRoot(path unsafe.Pointer, n int32) int32

// getCPUCount reads the online CPU count from /ctl/sched, which emits a
// "cpus: N" line (the Plan 9 /dev/sysstat shape). Allocation-free, so it is
// safe at osinit regardless of allocator-init order, and fail-soft: any error
// (no /ctl mount, short read, no "cpus:" line) yields 1.
func getCPUCount() int32 {
	path := [...]byte{'/', 'c', 't', 'l', '/', 's', 'c', 'h', 'e', 'd'}
	fd := openReadRoot(unsafe.Pointer(&path[0]), int32(len(path)))
	if fd < 0 {
		return 1
	}
	var buf [512]byte
	total := 0
	for total < len(buf) {
		n := read(int32(fd), unsafe.Pointer(&buf[total]), int32(len(buf)-total))
		if n <= 0 {
			break
		}
		total += int(n)
	}
	closefd(int32(fd))
	return parseCpusLine(buf[:total])
}

// parseCpusLine scans b for "cpus: N" and returns N (>= 1), without
// allocating. Returns 1 if the key or a valid number is absent.
func parseCpusLine(b []byte) int32 {
	for i := 0; i+5 <= len(b); i++ {
		if b[i] == 'c' && b[i+1] == 'p' && b[i+2] == 'u' && b[i+3] == 's' && b[i+4] == ':' {
			j := i + 5
			for j < len(b) && b[j] == ' ' {
				j++
			}
			n := int32(0)
			got := false
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				n = n*10 + int32(b[j]-'0')
				j++
				got = true
			}
			if got && n >= 1 {
				return n
			}
			return 1
		}
	}
	return 1
}

//go:nosplit
func getPageSize() uintptr {
	return physPageSize
}

//go:noescape
func readdirRaw(fd int32, p unsafe.Pointer, n int32) int32

// goenvs reads the per-Proc environment from the /env device (ARCH 9.7 / G15)
// into envs, the initial backing for os.Environ. The Plan 9 model: the
// environment is a directory of files (one /env/NAME per variable). We list
// /env via SYS_READDIR -- a plain read on the directory returns -1, so readdir
// is its enumeration path -- and read each /env/NAME value. Subsequent
// os.Setenv/Getenv operate on this cache without writing back (POSIX semantics),
// exactly as on plan9. Structurally this is the plan9 goenvs with the directory
// parse adapted to Thylacine's 9P2000.L Treaddir dirents (vs plan9's legacy
// stat-format read).
func goenvs() {
	envs = make([]string, 0, 16)

	dirpath := [...]byte{'/', 'e', 'n', 'v'}
	dirfd := openReadRoot(unsafe.Pointer(&dirpath[0]), int32(len(dirpath)))
	if dirfd < 0 {
		return // no /env mount (an early/unmounted Proc) -> empty environment
	}

	dbuf := new([4096]byte)       // a run of 9P2000.L dirents
	vbuf := new([4096]byte)       // a variable's value (<= ENV_VALUE_MAX)
	namebuf := make([]byte, 256)  // "/env/" + NAME, for the per-variable open
	copy(namebuf, "/env/")

	for {
		dn := readdirRaw(dirfd, unsafe.Pointer(&dbuf[0]), int32(len(dbuf)))
		if dn <= 0 {
			break
		}
		b := dbuf[:dn]
		// Each dirent: qid(13) + offset/cookie(8) + type(1) + namelen(2 LE) +
		// name(namelen). The kernel emits only whole entries per readdir call.
		for len(b) >= 24 {
			nlen := int(b[22]) | int(b[23])<<8
			if nlen <= 0 || 24+nlen > len(b) {
				break
			}
			name := b[24 : 24+nlen]
			if 5+nlen <= len(namebuf) {
				copy(namebuf[5:], name)
				fd := openReadRoot(unsafe.Pointer(&namebuf[0]), int32(5+nlen))
				if fd >= 0 {
					total := 0
					for total < len(vbuf) {
						r := read(fd, unsafe.Pointer(&vbuf[total]), int32(len(vbuf)-total))
						if r <= 0 {
							break
						}
						total += int(r)
					}
					closefd(fd)
					env := make([]byte, nlen+1+total)
					copy(env, name)
					env[nlen] = '='
					copy(env[nlen+1:], vbuf[:total])
					envs = append(envs, string(env))
				}
			}
			b = b[24+nlen:]
		}
	}
	closefd(dirfd)
}

func readRandom(r []byte) int {
	if len(r) == 0 {
		return 0
	}
	n := getrandom(unsafe.Pointer(&r[0]), uintptr(len(r)), 0)
	if n < 0 {
		return 0
	}
	return int(n)
}

// --- m lifecycle ---

func mpreinit(mp *m) {
	mp.gsignal = malg(32 * 1024)
	mp.gsignal.m = mp
}

func minit() {
	getg().m.procid = uint64(getpid())
}

func unminit() {}

func mdestroy(mp *m) {}

// newosproc creates a new OS thread for mp (the first hard divergence).
// Thylacine's SYS_THREAD_SPAWN is entry-point, not clone-resume: the child
// starts at thread_entry with x0 = mp, SP = mp.g0.stack.hi, every other
// register zeroed. thread_entry then sets g and enters mstart.
//
//go:nowritebarrier
func newosproc(mp *m) {
	stk := mp.g0.stack.hi
	ret := thread_spawn(abi.FuncPCABI0(thread_entry), stk, unsafe.Pointer(mp), 0, 0)
	if ret < 0 {
		print("runtime: failed to create new OS thread (have ", mcount(), " already; errno=", -ret, ")\n")
		throw("newosproc")
	}
}

//go:nosplit
func newosproc0(stacksize uintptr, fn unsafe.Pointer) {
	// Only the c-shared / c-archive buildmodes call this, neither of which
	// Thylacine supports.
	writeErrStr(failthreadcreate)
	exit(1)
}

// --- signals: stubbed (no POSIX signals / note delivery yet) ---

const preemptMSupported = false

func preemptM(mp *m) {
	// Not supported yet. Stage 2 models this with a note, the way POSIX OSes
	// use SIGURG.
}

func initsig(preinit bool) {}

func sigsave(p *sigset) {}

func msigrestore(sigmask sigset) {}

//go:nosplit
//go:nowritebarrierrec
func clearSignalHandlers() {}

func sigblock(exiting bool) {}

func setProcessCPUProfiler(hz int32) {}

func setThreadCPUProfiler(hz int32) {}

// signame has no signal-name table on Thylacine yet; panics that reach here
// report a bare number via the caller.
func signame(sig uint32) string {
	return ""
}

// sigenable / sigdisable / sigignore are the os/signal runtime hooks. No-ops
// until note delivery exists.
func sigenable(sig uint32)  {}
func sigdisable(sig uint32) {}
func sigignore(sig uint32)  {}

// sigpanic turns a fault that reached a Go context into a panic. Thylacine has
// no note delivery yet, so a userspace fault terminates the Proc in the kernel
// before it can reach here; this exists so the panic machinery links.
func sigpanic() {
	if !canpanic() {
		throw("unexpected signal during runtime execution")
	}
	throw("fault")
}

//go:nosplit
func crash() {
	*(*int32)(nil) = 0
}
