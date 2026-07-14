// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package os

import (
	"internal/poll"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// fixLongPath is a noop on non-Windows platforms.
func fixLongPath(path string) string {
	return path
}

// file is the real representation of *File. The extra level of indirection
// ensures that no clients of os can overwrite this data, which could cause
// the finalizer to close the wrong file descriptor.
type file struct {
	fdmu       poll.FDMutex
	sysfd      int
	name       string
	dirinfo    atomic.Pointer[dirInfo] // nil unless directory being read
	appendMode bool
}

// fd is the Thylacine implementation of Fd.
func (f *File) fd() uintptr {
	if f == nil {
		return ^(uintptr(0))
	}
	return uintptr(f.sysfd)
}

// newFileFromNewFile is called by NewFile.
func newFileFromNewFile(fd uintptr, name string) *File {
	fdi := int(fd)
	if fdi < 0 {
		return nil
	}
	f := &File{&file{sysfd: fdi, name: name}}
	runtime.SetFinalizer(f.file, (*file).close)
	return f
}

// Auxiliary information if the File describes a directory. buf holds a run of
// 9P2000.L dirents from SYS_READDIR.
type dirInfo struct {
	mu   sync.Mutex
	buf  [4096]byte // SYS_RW_MAX
	nbuf int        // length of valid buf
	bufp int        // location of next record in buf
}

func epipecheck(file *File, e error) {}

// DevNull is the name of the operating system's null device.
const DevNull = "/dev/null"

// syscallMode returns the syscall-specific mode bits from Go's portable mode
// bits. Thylacine carries only the POSIX permission bits.
func syscallMode(i FileMode) (o uint32) {
	o |= uint32(i.Perm())
	return
}

// openFileNolog is the Thylacine implementation of OpenFile.
func openFileNolog(name string, flag int, perm FileMode) (*File, error) {
	var (
		fd     int
		e      error
		create bool
		excl   bool
		appnd  bool
	)

	if flag&O_CREATE == O_CREATE {
		flag = flag &^ O_CREATE
		create = true
	}
	if flag&O_EXCL == O_EXCL {
		excl = true
	}
	// O_TRUNC is NOT stripped from flag: the kernel open folds it into the
	// omode word (omodeMask includes OTRUNC), so syscall.Open truncates an
	// existing file -- which is how the open-or-create path below honors
	// os.Create's truncate semantics without a separate ftruncate.
	// O_APPEND is emulated (seek to end after open).
	if flag&O_APPEND == O_APPEND {
		flag = flag &^ O_APPEND
		appnd = true
	}

	if excl {
		// O_CREATE|O_EXCL: must create a fresh file; an existing one is an error.
		fd, e = syscall.Create(name, flag, syscallMode(perm))
	} else if create {
		// O_CREATE without O_EXCL = POSIX open-or-create. Thylacine's
		// SYS_WALK_CREATE is bare-create (Tlcreate -> EEXIST on an existing
		// path), so trying to create an existing file fails; open it instead.
		// `flag` still carries O_TRUNC when requested, and the kernel open
		// honors it (dev9p_open maps OTRUNC -> Tlopen O_TRUNC), so an existing
		// file is truncated -- matching os.Create's contract. Create only when
		// the path does not yet exist. (The go-build compile re-creates
		// $WORK/*/go_asm.h, which is why this path is load-bearing.)
		fd, e = syscall.Open(name, flag)
		if IsNotExist(e) {
			fd, e = syscall.Create(name, flag, syscallMode(perm))
			if e != nil {
				return nil, &PathError{Op: "create", Path: name, Err: e}
			}
		}
	} else {
		fd, e = syscall.Open(name, flag)
	}

	if e != nil {
		return nil, &PathError{Op: "open", Path: name, Err: e}
	}

	if appnd {
		if _, e = syscall.Seek(fd, 0, io.SeekEnd); e != nil {
			return nil, &PathError{Op: "seek", Path: name, Err: e}
		}
	}

	f := NewFile(uintptr(fd), name)
	f.appendMode = appnd
	return f, nil
}

func openDirNolog(name string) (*File, error) {
	return openFileNolog(name, O_RDONLY, 0)
}

// Close closes the File, rendering it unusable for I/O.
func (f *File) Close() error {
	if f == nil {
		return ErrInvalid
	}
	return f.file.close()
}

func (file *file) close() error {
	if !file.fdmu.IncrefAndClose() {
		return &PathError{Op: "close", Path: file.name, Err: ErrClosed}
	}
	err := file.decref()
	runtime.SetFinalizer(file, nil)
	return err
}

// destroy actually closes the descriptor.
func (file *file) destroy() error {
	var err error
	if e := syscall.Close(file.sysfd); e != nil {
		err = &PathError{Op: "close", Path: file.name, Err: e}
	}
	return err
}

// Stat returns the FileInfo structure describing file.
func (f *File) Stat() (FileInfo, error) {
	if f == nil {
		return nil, ErrInvalid
	}
	if err := f.incref("stat"); err != nil {
		return nil, err
	}
	defer f.decref()
	var st syscall.Stat_t
	if err := syscall.Fstat(f.sysfd, &st); err != nil {
		return nil, &PathError{Op: "stat", Path: f.name, Err: err}
	}
	return fileInfoFromStat(&st, f.name), nil
}

// Truncate routes to the syscall layer: the Larder-served fstat NO-OP fast
// path (size == current -- cmd/go's putIndexEntry calls this on every cache
// write, #36 layer 3), then the real SYS_WSTAT(T_WSTAT_SIZE) truncation
// (Stage 5; the 9P Tsetattr size axis). This wrapper previously
// short-circuited ENOSYS WITHOUT consulting the syscall layer, so the
// layer-3 fix there was dead code and every put on an on-device go build
// kept self-deleting its index entry (#34 caught it: the gofmt warm build
// recompiled all 33 non-seed packages every time).
func (f *File) Truncate(size int64) error {
	if f == nil {
		return ErrInvalid
	}
	if err := f.incref("truncate"); err != nil {
		return err
	}
	defer f.decref()
	if e := syscall.Ftruncate(f.sysfd, size); e != nil {
		return &PathError{Op: "truncate", Path: f.name, Err: e}
	}
	return nil
}

func (f *File) chmod(mode FileMode) error {
	if f == nil {
		return ErrInvalid
	}
	if err := f.incref("chmod"); err != nil {
		return err
	}
	defer f.decref()
	if e := syscall.Fchmod(f.sysfd, syscallMode(mode)); e != nil {
		return &PathError{Op: "chmod", Path: f.name, Err: e}
	}
	return nil
}

// Sync commits the current contents of the file to stable storage.
func (f *File) Sync() error {
	if f == nil {
		return ErrInvalid
	}
	if err := f.incref("sync"); err != nil {
		return err
	}
	defer f.decref()
	if e := syscall.Fsync(f.sysfd); e != nil {
		return &PathError{Op: "sync", Path: f.name, Err: e}
	}
	return nil
}

func (f *File) read(b []byte) (n int, err error) {
	if err := f.readLock(); err != nil {
		return 0, err
	}
	defer f.readUnlock()
	n, e := fixCount(syscall.Read(f.sysfd, b))
	if n == 0 && len(b) > 0 && e == nil {
		return 0, io.EOF
	}
	return n, e
}

func (f *File) pread(b []byte, off int64) (n int, err error) {
	if err := f.readLock(); err != nil {
		return 0, err
	}
	defer f.readUnlock()
	n, e := fixCount(syscall.Pread(f.sysfd, b, off))
	if n == 0 && len(b) > 0 && e == nil {
		return 0, io.EOF
	}
	return n, e
}

func (f *File) write(b []byte) (n int, err error) {
	if err := f.writeLock(); err != nil {
		return 0, err
	}
	defer f.writeUnlock()
	// syscall.Write caps each call at SYS_RW_MAX and returns a partial count;
	// os.File.Write requires write() to consume all of b or report an error,
	// so loop until done -- the role internal/poll.FD.Write plays for regular
	// files on the other GOOSes. Without this loop a buffered writer over a
	// large object surfaces io.ErrShortWrite from bufio.Writer.Flush.
	for n < len(b) {
		m, e := fixCount(syscall.Write(f.sysfd, b[n:]))
		n += m
		if e != nil {
			return n, e
		}
		if m == 0 {
			return n, io.ErrUnexpectedEOF
		}
	}
	return n, nil
}

func (f *File) pwrite(b []byte, off int64) (n int, err error) {
	if err := f.writeLock(); err != nil {
		return 0, err
	}
	defer f.writeUnlock()
	// Same SYS_RW_MAX-capped partial-write loop as write(), positioned.
	for n < len(b) {
		m, e := fixCount(syscall.Pwrite(f.sysfd, b[n:], off+int64(n)))
		n += m
		if e != nil {
			return n, e
		}
		if m == 0 {
			return n, io.ErrUnexpectedEOF
		}
	}
	return n, nil
}

func (f *File) seek(offset int64, whence int) (ret int64, err error) {
	if err := f.incref(""); err != nil {
		return 0, err
	}
	defer f.decref()
	// Free cached dirinfo, so we reallocate if this file is read as a
	// directory again.
	f.dirinfo.Store(nil)
	return syscall.Seek(f.sysfd, offset, whence)
}

// Truncate of a named file: open O_WRONLY + the fd-level Truncate (the
// write-open carries the RIGHT_WRITE the kernel's T_WSTAT_SIZE gate
// demands, and the open-time perm_check is the POSIX W-permission check).
func Truncate(name string, size int64) error {
	f, err := OpenFile(name, O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Truncate(size)
}

// Remove removes the named file or directory. Like the Unix os.Remove, it
// tries unlink first (SYS_UNLINK) and falls back to rmdir (SYS_UNLINK with
// the REMOVEDIR flag) -- there is no single "remove either" kernel call, and
// the caller does not say whether name is a directory.
func Remove(name string) error {
	e := syscall.Remove(name)
	if e == nil {
		return nil
	}
	// Could be a directory: retry as rmdir. If that succeeds, done.
	if e1 := syscall.Rmdir(name); e1 == nil {
		return nil
	}
	return &PathError{Op: "remove", Path: name, Err: e}
}

func rename(oldname, newname string) error {
	// SYS_RENAME resolves both parent directories and works across
	// directories within one Dev.
	if e := syscall.Rename(oldname, newname); e != nil {
		return &LinkError{"rename", oldname, newname, e}
	}
	return nil
}

func chmod(name string, mode FileMode) error {
	if e := syscall.Chmod(name, syscallMode(mode)); e != nil {
		return &PathError{Op: "chmod", Path: name, Err: e}
	}
	return nil
}

// Chtimes has no v1.0 kernel surface (no utime).
func Chtimes(name string, atime time.Time, mtime time.Time) error {
	return &PathError{Op: "chtimes", Path: name, Err: syscall.ENOSYS}
}

// Pipe returns a connected pair of Files.
func Pipe() (r *File, w *File, err error) {
	var p [2]int
	if e := syscall.Pipe(p[0:]); e != nil {
		return nil, nil, NewSyscallError("pipe", e)
	}
	return NewFile(uintptr(p[0]), "|0"), NewFile(uintptr(p[1]), "|1"), nil
}

// Link / Symlink / readlink are unsupported at v1.0 (no link surface, G11).
func Link(oldname, newname string) error {
	return &LinkError{"link", oldname, newname, syscall.ENOSYS}
}

func Symlink(oldname, newname string) error {
	return &LinkError{"symlink", oldname, newname, syscall.ENOSYS}
}

func readlink(name string) (string, error) {
	return "", &PathError{Op: "readlink", Path: name, Err: syscall.ENOSYS}
}

// Chown / Lchown are not supported by name at v1.0 (chown rides SYS_WSTAT on an
// open fd, gated by CAP_CHOWN -- not wired through the os layer yet).
func Chown(name string, uid, gid int) error {
	return &PathError{Op: "chown", Path: name, Err: syscall.ENOSYS}
}

func Lchown(name string, uid, gid int) error {
	return &PathError{Op: "lchown", Path: name, Err: syscall.ENOSYS}
}

func (f *File) Chown(uid, gid int) error {
	if f == nil {
		return ErrInvalid
	}
	return &PathError{Op: "chown", Path: f.name, Err: syscall.ENOSYS}
}

func tempDir() string {
	dir := Getenv("TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	return dir
}

// Chdir changes the current working directory to the file.
func (f *File) Chdir() error {
	if err := f.incref("chdir"); err != nil {
		return err
	}
	defer f.decref()
	if e := syscall.Fchdir(f.sysfd); e != nil {
		return &PathError{Op: "chdir", Path: f.name, Err: e}
	}
	return nil
}

// setDeadline / setReadDeadline / setWriteDeadline: deadlines are not honored
// for ordinary files (like Plan 9).
func (f *File) setDeadline(time.Time) error {
	if err := f.checkValid("SetDeadline"); err != nil {
		return err
	}
	return poll.ErrNoDeadline
}

func (f *File) setReadDeadline(time.Time) error {
	if err := f.checkValid("SetReadDeadline"); err != nil {
		return err
	}
	return poll.ErrNoDeadline
}

func (f *File) setWriteDeadline(time.Time) error {
	if err := f.checkValid("SetWriteDeadline"); err != nil {
		return err
	}
	return poll.ErrNoDeadline
}

// checkValid checks whether f is valid for use.
func (f *File) checkValid(op string) error {
	if f == nil {
		return ErrInvalid
	}
	if err := f.incref(op); err != nil {
		return err
	}
	return f.decref()
}

type rawConn struct{}

func (c *rawConn) Control(f func(uintptr)) error { return syscall.ENOSYS }
func (c *rawConn) Read(f func(uintptr) bool) error { return syscall.ENOSYS }
func (c *rawConn) Write(f func(uintptr) bool) error { return syscall.ENOSYS }

func newRawConn(file *File) (*rawConn, error) {
	return nil, syscall.ENOSYS
}

func ignoringEINTR(fn func() error) error {
	return fn()
}

func ignoringEINTR2[T any](fn func() (T, error)) (T, error) {
	return fn()
}
