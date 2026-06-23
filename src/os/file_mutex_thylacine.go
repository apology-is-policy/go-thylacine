// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package os

// File locking support for Thylacine, using fdMutex from internal/poll (like
// Plan 9). Ordinary file I/O goes straight through syscall; only the locking
// semantics are borrowed.

func (f *File) incref(op string) (err error) {
	if f == nil {
		return ErrInvalid
	}
	if !f.fdmu.Incref() {
		err = ErrClosed
		if op != "" {
			err = &PathError{Op: op, Path: f.name, Err: err}
		}
	}
	return err
}

func (file *file) decref() error {
	if file.fdmu.Decref() {
		return file.destroy()
	}
	return nil
}

func (file *file) readLock() error {
	if !file.fdmu.ReadLock() {
		return ErrClosed
	}
	return nil
}

func (file *file) readUnlock() {
	if file.fdmu.ReadUnlock() {
		file.destroy()
	}
}

func (file *file) writeLock() error {
	if !file.fdmu.WriteLock() {
		return ErrClosed
	}
	return nil
}

func (file *file) writeUnlock() {
	if file.fdmu.WriteUnlock() {
		file.destroy()
	}
}
