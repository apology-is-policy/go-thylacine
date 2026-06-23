// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package os

import (
	"internal/itoa"
	"syscall"
	"time"
)

// The only signal values guaranteed present in the os package on all systems
// are Interrupt and Kill. On Thylacine they are notes.
var (
	Interrupt Signal = syscall.Note("interrupt")
	Kill      Signal = syscall.Note("kill")
)

func startProcess(name string, argv []string, attr *ProcAttr) (p *Process, err error) {
	sysattr := &syscall.ProcAttr{
		Dir: attr.Dir,
		Env: attr.Env,
		Sys: attr.Sys,
	}

	sysattr.Files = make([]uintptr, 0, len(attr.Files))
	for _, f := range attr.Files {
		sysattr.Files = append(sysattr.Files, f.Fd())
	}

	pid, _, e := syscall.StartProcess(name, argv, sysattr)
	if e != nil {
		return nil, &PathError{Op: "fork/exec", Path: name, Err: e}
	}

	return newPIDProcess(pid), nil
}

func (p *Process) writeProcFile(file string, data string) error {
	f, e := OpenFile("/proc/"+itoa.Itoa(p.Pid)+"/"+file, O_WRONLY, 0)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.Write([]byte(data))
	return e
}

func (p *Process) signal(sig Signal) error {
	switch p.pidStatus() {
	case statusDone:
		return ErrProcessDone
	case statusReleased:
		return syscall.ENOENT
	}

	// /proc/<pid>/ctl accepts "kill"/"killgrp" (I-26). A signal whose note
	// name the kernel does not recognize as a ctl verb returns an error.
	verb := sig.String()
	if verb == "kill" {
		// keep as-is
	}
	if e := p.writeProcFile("ctl", verb); e != nil {
		return NewSyscallError("signal", e)
	}
	return nil
}

func (p *Process) kill() error {
	return p.signal(Kill)
}

func (p *Process) wait() (ps *ProcessState, err error) {
	var waitmsg syscall.Waitmsg

	switch p.pidStatus() {
	case statusReleased:
		return nil, ErrInvalid
	}

	err = syscall.WaitProcess(p.Pid, &waitmsg)
	if err != nil {
		return nil, NewSyscallError("wait", err)
	}

	p.doRelease(statusDone)
	ps = &ProcessState{
		pid:    waitmsg.Pid,
		status: &waitmsg,
	}
	return ps, nil
}

func findProcess(pid int) (p *Process, err error) {
	// NOOP for Thylacine.
	return newPIDProcess(pid), nil
}

// ProcessState stores information about a process, as reported by Wait.
type ProcessState struct {
	pid    int
	status *syscall.Waitmsg
}

// Pid returns the process id of the exited process.
func (p *ProcessState) Pid() int {
	return p.pid
}

func (p *ProcessState) exited() bool {
	return p.status.Exited()
}

func (p *ProcessState) success() bool {
	return p.status.ExitStatus() == 0
}

func (p *ProcessState) sys() any {
	return p.status
}

func (p *ProcessState) sysUsage() any {
	return p.status
}

func (p *ProcessState) userTime() time.Duration {
	return time.Duration(p.status.Time[0]) * time.Millisecond
}

func (p *ProcessState) systemTime() time.Duration {
	return time.Duration(p.status.Time[1]) * time.Millisecond
}

func (p *ProcessState) String() string {
	if p == nil {
		return "<nil>"
	}
	if p.status.ExitStatus() == 0 {
		return "exit status: 0"
	}
	return "exit status: " + itoa.Itoa(p.status.ExitStatus())
}

// ExitCode returns the exit code of the exited process, or -1 if the process
// hasn't exited or was terminated by a signal.
func (p *ProcessState) ExitCode() int {
	if p == nil {
		return -1
	}
	return p.status.ExitStatus()
}
