// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package syscall

// Errno values, POSIX-aligned (kernel/include/thylacine/errno.h pins these
// to the standard numbers). The kernel returns them as a negative value in
// x0; the asm decode negates back to a positive Errno.
const (
	EPERM         Errno = 1
	ENOENT        Errno = 2
	ESRCH         Errno = 3
	EINTR         Errno = 4
	EIO           Errno = 5
	ENXIO         Errno = 6
	E2BIG         Errno = 7
	ENOEXEC       Errno = 8
	EBADF         Errno = 9
	ECHILD        Errno = 10
	EAGAIN        Errno = 11
	ENOMEM        Errno = 12
	EACCES        Errno = 13
	EFAULT        Errno = 14
	EBUSY         Errno = 16
	EEXIST        Errno = 17
	EXDEV         Errno = 18
	ENODEV        Errno = 19
	ENOTDIR       Errno = 20
	EISDIR        Errno = 21
	EINVAL        Errno = 22
	ENFILE        Errno = 23
	EMFILE        Errno = 24
	ENOTTY        Errno = 25
	ETXTBSY       Errno = 26
	EFBIG         Errno = 27
	ENOSPC        Errno = 28
	ESPIPE        Errno = 29
	EROFS         Errno = 30
	EMLINK        Errno = 31
	EPIPE         Errno = 32
	ERANGE        Errno = 34
	ENAMETOOLONG  Errno = 36
	ENOSYS        Errno = 38
	ENOTEMPTY     Errno = 39
	ELOOP         Errno = 40
	EOVERFLOW     Errno = 75
	EOPNOTSUPP    Errno = 95
	EAFNOSUPPORT  Errno = 97
	EADDRINUSE    Errno = 98
	ECONNREFUSED  Errno = 111
	ETIMEDOUT     Errno = 110
	ECANCELED     Errno = 125
	EWOULDBLOCK   = EAGAIN
	ENOTSUP       = EOPNOTSUPP
	EACCESS       = EACCES
)

var errorNames = map[Errno]string{
	EPERM:        "operation not permitted",
	ENOENT:       "no such file or directory",
	ESRCH:        "no such process",
	EINTR:        "interrupted system call",
	EIO:          "input/output error",
	ENXIO:        "no such device or address",
	E2BIG:        "argument list too long",
	ENOEXEC:      "exec format error",
	EBADF:        "bad file descriptor",
	ECHILD:       "no child processes",
	EAGAIN:       "resource temporarily unavailable",
	ENOMEM:       "cannot allocate memory",
	EACCES:       "permission denied",
	EFAULT:       "bad address",
	EBUSY:        "device or resource busy",
	EEXIST:       "file exists",
	EXDEV:        "invalid cross-device link",
	ENODEV:       "no such device",
	ENOTDIR:      "not a directory",
	EISDIR:       "is a directory",
	EINVAL:       "invalid argument",
	ENFILE:       "too many open files in system",
	EMFILE:       "too many open files",
	ENOTTY:       "inappropriate ioctl for device",
	ETXTBSY:      "text file busy",
	EFBIG:        "file too large",
	ENOSPC:       "no space left on device",
	ESPIPE:       "illegal seek",
	EROFS:        "read-only file system",
	EMLINK:       "too many links",
	EPIPE:        "broken pipe",
	ERANGE:       "numerical result out of range",
	ENAMETOOLONG: "file name too long",
	ENOSYS:       "function not implemented",
	ENOTEMPTY:    "directory not empty",
	ELOOP:        "too many levels of symbolic links",
	EOVERFLOW:    "value too large for defined data type",
	EOPNOTSUPP:   "operation not supported",
	EAFNOSUPPORT: "address family not supported by protocol",
	EADDRINUSE:   "address already in use",
	ECONNREFUSED: "connection refused",
	ETIMEDOUT:    "connection timed out",
	ECANCELED:    "operation canceled",
}

// A Note is a string describing a process note. It implements os.Signal,
// the way Plan 9's syscall.Note does. Thylacine notes (SYS_POSTNOTE) are
// the same model.
type Note string

func (n Note) Signal()        {}
func (n Note) String() string { return string(n) }

// Signal is an alias so code that names syscall.Signal compiles; on
// Thylacine the concrete signal value is a Note.
type Signal = Note

var (
	// SIGINT / SIGKILL exposed for os.Interrupt / os.Kill.
	Interrupt Signal = Note("interrupt")
	Kill      Signal = Note("kill")
	SIGINT    Signal = Note("interrupt")
	SIGKILL   Signal = Note("kill")
	SIGTERM   Signal = Note("terminate")
)
