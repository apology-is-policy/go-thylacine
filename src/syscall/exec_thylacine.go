// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package syscall

import (
	"internal/itoa"
	"runtime"
	"sync"
	"unsafe"
)

// spawnDirMu serializes the chdir-spawn-restore dance in startProcess that
// honors ProcAttr.Dir. SYS_SPAWN_FULL_ARGV has no working-directory argument:
// the child inherits the parent's cwd (the territory_clone deep-copies dot_path
// at rfork). So a caller-set Dir is applied by chdir'ing the PARENT to Dir,
// spawning -- the child captures cwd=Dir at clone -- then restoring the parent's
// cwd. The mutex keeps concurrent StartProcess calls (cmd/go runs the
// compile/asm/link tools with Dir=$WORK, in parallel) from racing the per-Proc
// cwd. The serialized window is only getwd+chdir+spawn+chdir; the child's actual
// run is NOT serialized (the spawn syscall returns once the child exists), so
// build parallelism is preserved. This is the entry-point-spawn analog of
// Plan 9's chdir-in-the-rfork-child (thylacine's child is already the new image,
// so the chdir must happen in the parent across an atomic spawn window).
var spawnDirMu sync.Mutex

// Process spawning, Thylacine. Like Plan 9, there is no Unix fork: a child is
// created fully-formed by SYS_SPAWN_FULL_ARGV (name + argv + inherited fds),
// and reaped by SYS_WAIT_PID. Both ride the entersyscall-wrapped Syscall
// primitive -- a blocking SYS_WAIT_PID must leave the goroutine in _Gsyscall
// so a concurrent GC stop-the-world is not wedged on the un-preemptible SVC.

// SysProcAttr holds optional, OS-specific attributes for StartProcess. There
// is no fork-time child setup on Thylacine beyond the inherited fd list, so it
// is empty at v1.0.
type SysProcAttr struct{}

// Exec is the Unix execve(2) -- replace the running image in place. Thylacine
// is spawn-only (SYS_SPAWN_FULL_ARGV creates a fresh child; there is no
// exec-in-place), so Exec is unsupported and returns ENOSYS. It exists for
// source compatibility: cmd/link's execArchive (external linking) references
// it, but a pure-Go CGO_ENABLED=0 build always links internally, so the path is
// never taken at runtime.
func Exec(argv0 string, argv []string, envv []string) (err error) {
	return ENOSYS
}

// ProcAttr holds attributes that will be applied to a new process started by
// StartProcess.
type ProcAttr struct {
	Dir   string
	Env   []string
	Files []uintptr
	Sys   *SysProcAttr
}

// Waitmsg stores the information about an exited process as reported by Wait.
// Msg is empty on a clean (status 0) exit, like Plan 9's wait message.
type Waitmsg struct {
	Pid    int
	Time   [3]uint32
	Msg    string
	Status int // the raw exit status SYS_WAIT_PID reported
}

func (w Waitmsg) Exited() bool   { return true }
func (w Waitmsg) Signaled() bool { return false }

func (w Waitmsg) ExitStatus() int {
	return w.Status
}

func StartProcess(argv0 string, argv []string, attr *ProcAttr) (pid int, handle uintptr, err error) {
	return startProcess(argv0, argv, attr)
}

func WaitProcess(pid int, w *Waitmsg) error {
	return waitProcess(pid, w)
}

// The SYS_SPAWN_FULL_ARGV bound constants (spawnNameMax / spawnArgvMax /
// spawnArgvDataMax / spawnMaxFds) live in const_thylacine.go.

// spawnArgs mirrors struct sys_spawn_args (the 96-byte SYS_SPAWN_FULL_ARGV
// record). Every u64 lands on an 8-byte boundary, so Go's natural struct
// layout matches the kernel's byte-for-byte (verified against the kernel's
// _Static_assert offset pins). The trailing identity (56..80) and allowance
// (80..96) blocks are left zero -- the child inherits the parent's identity
// and broad allowance, exactly as every C caller that zero-fills the struct.
type spawnArgs struct {
	nameVa         uint64 // 0
	argvDataVa     uint64 // 8
	fdListVa       uint64 // 16
	nameLen        uint32 // 24
	argvDataLen    uint32 // 28
	argc           uint32 // 32
	fdCount        uint32 // 36
	permFlags      uint32 // 40
	padEnvp        uint32 // 44
	capMask        uint64 // 48
	principalID    uint32 // 56
	primaryGid     uint32 // 60
	suppGidsVa     uint64 // 64
	suppGidCount   uint32 // 72
	identityFlags  uint32 // 76
	allowanceVa    uint64 // 80
	allowanceFlags uint32 // 88
	padAllow       uint32 // 92
}

func startProcess(argv0 string, argv []string, attr *ProcAttr) (pid int, handle uintptr, err error) {
	if len(argv0) == 0 || len(argv0) > spawnNameMax {
		return 0, 0, EINVAL
	}
	// Env is silently dropped: Thylacine native processes inherit the parent's
	// /env (G15), and os/exec always populates ProcAttr.Env, so erroring on it
	// is wrong. ProcAttr.Dir is honored below (chdir-spawn-restore around the
	// SYS_SPAWN_FULL_ARGV call); cmd/go runs the compile/asm/link tools with
	// Dir=$WORK, so dropping it would build in the wrong directory.

	// argv buffer: each entry NUL-terminated; argc = entry count. The kernel
	// delivers argv = [argv0, args...] verbatim (argv[0] is included in the
	// buffer). os.StartProcess already puts the program name at argv[0]; if a
	// caller passed an empty argv, synthesize argv[0] = argv0.
	var argvBuf []byte
	argc := 0
	if len(argv) == 0 {
		argvBuf = append(argvBuf, argv0...)
		argvBuf = append(argvBuf, 0)
		argc = 1
	} else {
		for _, a := range argv {
			argvBuf = append(argvBuf, a...)
			argvBuf = append(argvBuf, 0)
			argc++
		}
	}
	if argc > spawnArgvMax {
		return 0, 0, EINVAL
	}
	if len(argvBuf) > spawnArgvDataMax {
		return 0, 0, EINVAL
	}

	// fd_list: the child receives these handles at slots 0..n-1
	// (Files[0]=stdin, Files[1]=stdout, Files[2]=stderr, then any extras).
	var fds []uint32
	if attr != nil {
		fds = make([]uint32, 0, len(attr.Files))
		for _, f := range attr.Files {
			fds = append(fds, uint32(f))
		}
	}
	if len(fds) > spawnMaxFds {
		return 0, 0, EINVAL
	}

	name := []byte(argv0)
	var fdListVa uint64
	if len(fds) > 0 {
		fdListVa = uint64(uintptr(unsafe.Pointer(&fds[0])))
	}
	rec := spawnArgs{
		nameVa:      uint64(uintptr(unsafe.Pointer(&name[0]))),
		argvDataVa:  uint64(uintptr(unsafe.Pointer(&argvBuf[0]))),
		fdListVa:    fdListVa,
		nameLen:     uint32(len(name)),
		argvDataLen: uint32(len(argvBuf)),
		argc:        uint32(argc),
		fdCount:     uint32(len(fds)),
	}
	// Honor ProcAttr.Dir: chdir the parent to Dir so the spawned child inherits
	// cwd=Dir, then restore. Serialized by spawnDirMu so concurrent spawns do
	// not race the per-Proc cwd; the deferred restore + unlock run at function
	// return, just after the spawn syscall has captured the child's cwd.
	if attr != nil && attr.Dir != "" {
		spawnDirMu.Lock()
		defer spawnDirMu.Unlock()
		saved, gerr := Getwd()
		if gerr != nil {
			return 0, 0, gerr
		}
		if cerr := Chdir(attr.Dir); cerr != nil {
			return 0, 0, cerr
		}
		defer Chdir(saved)
	}
	r1, _, e := Syscall(SYS_SPAWN_FULL_ARGV, uintptr(unsafe.Pointer(&rec)), 0, 0)
	// The kernel copies name/argv_data/fd_list before returning; keep the
	// backing buffers alive across the SVC.
	runtime.KeepAlive(name)
	runtime.KeepAlive(argvBuf)
	runtime.KeepAlive(fds)
	if e != 0 {
		return 0, 0, e
	}
	return int(r1), 0, nil
}

func waitProcess(pid int, w *Waitmsg) error {
	// want_pid = pid (>0 selects that child), flags = 0 (block), status_out =
	// &status. The kernel writes the child's exit_status to *status_out and
	// returns the reaped pid. SYS_WAIT_PID blocks, so Syscall's entersyscall
	// wrap is load-bearing here.
	var status int32
	r1, _, e := Syscall(SYS_WAIT_PID, uintptr(pid), 0, uintptr(unsafe.Pointer(&status)))
	if e != 0 {
		return e
	}
	if w != nil {
		w.Pid = int(r1)
		w.Status = int(status)
		if status != 0 {
			w.Msg = "exit status " + itoa.Itoa(int(status))
		}
	}
	return nil
}
