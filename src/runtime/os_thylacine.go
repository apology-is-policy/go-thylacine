// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
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
// torpor_wait(addr, expected, timeout_us): if *addr == expected, sleep up to
// timeout_us microseconds; timeout_us == 0 (and < 0) means "forever". For a
// BOUNDED Go sleep we must never pass 0 (that would block forever), so a
// computed-zero microsecond count is clamped up to 1.

//go:nosplit
func futexsleep(addr *uint32, val uint32, ns int64) {
	if ns < 0 {
		torpor_wait(unsafe.Pointer(addr), val, 0) // forever
		return
	}
	us := ns / 1000
	if us == 0 {
		us = 1 // 0 microseconds would mean "forever" to the kernel
	}
	torpor_wait(unsafe.Pointer(addr), val, us)
}

//go:nosplit
func futexwakeup(addr *uint32, cnt uint32) {
	torpor_wake(unsafe.Pointer(addr), cnt)
}

// sleepDummy backs osyield and usleep. It is never written or woken, so a
// matching-value wait always times out and a mismatching-value wait returns
// at once.
var sleepDummy uint32

//go:nosplit
func osyield() {
	// No yield syscall. *sleepDummy == 0 != 1, so this returns immediately
	// after a syscall round-trip -- a scheduling point, which is the point.
	torpor_wait(unsafe.Pointer(&sleepDummy), 1, 1)
}

//go:nosplit
func osyield_no_g() {
	osyield()
}

//go:nosplit
func usleep(us uint32) {
	u := int64(us)
	if u == 0 {
		u = 1 // 0 would mean "forever" to the kernel
	}
	// *sleepDummy == 0 == 0, so this parks for up to u microseconds (no waker).
	torpor_wait(unsafe.Pointer(&sleepDummy), 0, u)
}

//go:nosplit
func usleep_no_g(us uint32) {
	usleep(us)
}

// --- time ---

//go:nosplit
func nanotime1() int64 {
	var ts timespec
	clock_gettime(_CLOCK_MONOTONIC, &ts)
	return ts.tv_sec*1e9 + ts.tv_nsec
}

//go:nosplit
func walltime() (sec int64, nsec int32) {
	var ts timespec
	clock_gettime(_CLOCK_REALTIME, &ts)
	return ts.tv_sec, int32(ts.tv_nsec)
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
