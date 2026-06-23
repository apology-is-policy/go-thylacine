// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

// The Thylacine FD mirrors the Plan 9 one: ordinary file I/O in the os
// package goes straight through syscall and uses only FDMutex; this FD type
// (with its deadline machinery) backs the net package. Deadline cancellation
// is a no-op at v1.0 (Stage 3c will wire a real interrupt); blocking I/O is
// correct, deadlines simply do not abort an in-flight call yet.

package poll

import (
	"errors"
	"internal/stringslite"
	"io"
	"sync"
	"syscall"
	"time"
)

type FD struct {
	// Lock sysfd and serialize access to Read and Write methods.
	fdmu fdMutex

	Destroy func()

	// deadlines
	rmu       sync.Mutex
	wmu       sync.Mutex
	raio      *asyncIO
	waio      *asyncIO
	rtimer    *time.Timer
	wtimer    *time.Timer
	rtimedout bool
	wtimedout bool

	// Whether this is a normal file. We do not use this package for
	// ordinary files (like Plan 9), so this is always false; the field is
	// present because shared code in fd_mutex.go checks it.
	isFile bool
}

func (fd *FD) destroy() error {
	if fd.Destroy != nil {
		fd.Destroy()
	}
	return nil
}

// Close handles the locking for closing an FD. The real operation is in the
// net package.
func (fd *FD) Close() error {
	if !fd.fdmu.increfAndClose() {
		return errClosing(fd.isFile)
	}
	return nil
}

// Read implements io.Reader.
func (fd *FD) Read(fn func([]byte) (int, error), b []byte) (int, error) {
	if err := fd.readLock(); err != nil {
		return 0, err
	}
	defer fd.readUnlock()
	if len(b) == 0 {
		return 0, nil
	}
	fd.rmu.Lock()
	if fd.rtimedout {
		fd.rmu.Unlock()
		return 0, ErrDeadlineExceeded
	}
	fd.raio = newAsyncIO(fn, b)
	fd.rmu.Unlock()
	n, err := fd.raio.Wait()
	fd.raio = nil
	if isHangup(err) {
		err = io.EOF
	}
	if isInterrupted(err) {
		err = ErrDeadlineExceeded
	}
	return n, err
}

// Write implements io.Writer.
func (fd *FD) Write(fn func([]byte) (int, error), b []byte) (int, error) {
	if err := fd.writeLock(); err != nil {
		return 0, err
	}
	defer fd.writeUnlock()
	fd.wmu.Lock()
	if fd.wtimedout {
		fd.wmu.Unlock()
		return 0, ErrDeadlineExceeded
	}
	fd.waio = newAsyncIO(fn, b)
	fd.wmu.Unlock()
	n, err := fd.waio.Wait()
	fd.waio = nil
	if isInterrupted(err) {
		err = ErrDeadlineExceeded
	}
	return n, err
}

// SetDeadline sets the read and write deadlines associated with fd.
func (fd *FD) SetDeadline(t time.Time) error {
	return setDeadlineImpl(fd, t, 'r'+'w')
}

func (fd *FD) SetReadDeadline(t time.Time) error {
	return setDeadlineImpl(fd, t, 'r')
}

func (fd *FD) SetWriteDeadline(t time.Time) error {
	return setDeadlineImpl(fd, t, 'w')
}

func setDeadlineImpl(fd *FD, t time.Time, mode int) error {
	d := t.Sub(time.Now())
	if mode == 'r' || mode == 'r'+'w' {
		fd.rmu.Lock()
		defer fd.rmu.Unlock()
		if fd.rtimer != nil {
			fd.rtimer.Stop()
			fd.rtimer = nil
		}
		fd.rtimedout = false
	}
	if mode == 'w' || mode == 'r'+'w' {
		fd.wmu.Lock()
		defer fd.wmu.Unlock()
		if fd.wtimer != nil {
			fd.wtimer.Stop()
			fd.wtimer = nil
		}
		fd.wtimedout = false
	}
	if !t.IsZero() && d > 0 {
		if mode == 'r' || mode == 'r'+'w' {
			var timer *time.Timer
			timer = time.AfterFunc(d, func() {
				fd.rmu.Lock()
				defer fd.rmu.Unlock()
				if fd.rtimer != timer {
					return
				}
				fd.rtimedout = true
				if fd.raio != nil {
					fd.raio.Cancel()
				}
			})
			fd.rtimer = timer
		}
		if mode == 'w' || mode == 'r'+'w' {
			var timer *time.Timer
			timer = time.AfterFunc(d, func() {
				fd.wmu.Lock()
				defer fd.wmu.Unlock()
				if fd.wtimer != timer {
					return
				}
				fd.wtimedout = true
				if fd.waio != nil {
					fd.waio.Cancel()
				}
			})
			fd.wtimer = timer
		}
	}
	if !t.IsZero() && d <= 0 {
		if mode == 'r' || mode == 'r'+'w' {
			fd.rtimedout = true
			if fd.raio != nil {
				fd.raio.Cancel()
			}
		}
		if mode == 'w' || mode == 'r'+'w' {
			fd.wtimedout = true
			if fd.waio != nil {
				fd.waio.Cancel()
			}
		}
	}
	return nil
}

// ReadLock wraps FD.readLock for the net code.
func (fd *FD) ReadLock() error {
	return fd.readLock()
}

func (fd *FD) ReadUnlock() {
	fd.readUnlock()
}

func isHangup(err error) bool {
	return err != nil && stringslite.HasSuffix(err.Error(), "Hangup")
}

func isInterrupted(err error) bool {
	return err != nil && stringslite.HasSuffix(err.Error(), "interrupted")
}

// IsPollDescriptor reports whether fd is the descriptor being used by the
// poller. Only used for testing.
func IsPollDescriptor(fd uintptr) bool {
	return false
}

func (fd *FD) RawControl(f func(uintptr)) error {
	return errors.New("not implemented")
}

func (fd *FD) RawRead(f func(uintptr) bool) error {
	return errors.New("not implemented")
}

func (fd *FD) RawWrite(f func(uintptr) bool) error {
	return errors.New("not implemented")
}

func DupCloseOnExec(fd int) (int, string, error) {
	nfd, err := syscall.Dup(int(fd))
	if err != nil {
		return 0, "dup", err
	}
	// Thylacine spawn installs only the fds listed in SYS_SPAWN_FULL_ARGV,
	// so there is no separate close-on-exec flag to set.
	return nfd, "", nil
}
