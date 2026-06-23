// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package syscall

// Process spawning, Thylacine. Like Plan 9, there is no Unix fork: a child is
// created fully-formed by SYS_SPAWN_FULL_ARGV (name + argv + inherited fds).
// The real StartProcess / WaitProcess land in Stage 3b; here are the types the
// os package needs plus the v1.0 stubs.

// SysProcAttr holds optional, OS-specific attributes for StartProcess. There
// is no fork-time child setup on Thylacine beyond the inherited fd list, so it
// is empty at v1.0.
type SysProcAttr struct{}

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

// StartProcess and WaitProcess are filled in at Stage 3b (SYS_SPAWN_FULL_ARGV
// and SYS_WAIT_PID respectively). Stage 3a only needs them to exist so the os
// package compiles.

func StartProcess(argv0 string, argv []string, attr *ProcAttr) (pid int, handle uintptr, err error) {
	return startProcess(argv0, argv, attr)
}

func WaitProcess(pid int, w *Waitmsg) error {
	return waitProcess(pid, w)
}

func startProcess(argv0 string, argv []string, attr *ProcAttr) (pid int, handle uintptr, err error) {
	return 0, 0, ENOSYS
}

func waitProcess(pid int, w *Waitmsg) error {
	return ENOSYS
}
