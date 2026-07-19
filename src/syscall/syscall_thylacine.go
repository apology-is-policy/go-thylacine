// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

// Package syscall, Thylacine port. The shape follows Plan 9 (a non-Unix
// GOOS: Open and Create are distinct, files are reached by path through the
// per-Proc namespace, there is no fork), but errors are numeric Errno values
// (the kernel returns -errno, Linux-style) rather than Plan 9 error strings,
// and stat is a fixed-layout struct (SYS_FSTAT/SYS_STAT fills an 88-byte
// t_stat) rather than a marshaled 9P Dir.

package syscall

import (
	"errors"
	"internal/itoa"
	"internal/oserror"
	"runtime"
	"unsafe"
)

const ImplementsGetwd = true

var (
	Stdin  = 0
	Stdout = 1
	Stderr = 2
)

// An Errno is an unsigned number describing an error condition. The kernel
// returns it negated in x0; the asm decode negates it back to positive.
type Errno uintptr

func (e Errno) Error() string {
	if s, ok := errorNames[e]; ok {
		return s
	}
	return "errno " + itoa.Itoa(int(e))
}

func (e Errno) Is(target error) bool {
	switch target {
	case oserror.ErrPermission:
		return e == EACCES || e == EPERM
	case oserror.ErrExist:
		return e == EEXIST || e == ENOTEMPTY
	case oserror.ErrNotExist:
		return e == ENOENT
	case errors.ErrUnsupported:
		return e == ENOSYS || e == EOPNOTSUPP
	}
	return false
}

func (e Errno) Temporary() bool {
	return e == EINTR || e == EMFILE || e == ENFILE || e.Timeout()
}

func (e Errno) Timeout() bool {
	return e == EAGAIN || e == ETIMEDOUT
}

// The asm syscall primitives (asm_thylacine_arm64.s).
func Syscall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno)
func Syscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno)
func RawSyscall(trap, a1, a2, a3 uintptr) (r1, r2, err uintptr)
func RawSyscall6(trap, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2, err uintptr)

// Time types match struct t_timespec (i64 sec, i64 nsec).
type Timespec struct {
	Sec  int64
	Nsec int64
}

type Timeval struct {
	Sec  int64
	Usec int64
}

func NsecToTimespec(nsec int64) Timespec {
	return Timespec{Sec: nsec / 1e9, Nsec: nsec % 1e9}
}

func NsecToTimeval(nsec int64) Timeval {
	nsec += 999 // round up to microsecond
	return Timeval{Sec: nsec / 1e9, Usec: nsec % 1e9 / 1e3}
}

// Stat_t is the fixed 88-byte SYS_FSTAT/SYS_STAT record (struct t_stat in
// kernel/include/thylacine/syscall.h). Plan 9 qid identity carried verbatim
// alongside POSIX-shaped mode/size/time. The kernel writes sizeof(Stat_t)
// bytes into this struct via unsafe.Pointer, so the Go layout IS the ABI --
// it must track the kernel t_stat byte-for-byte and grow in lockstep, else a
// stale (smaller) struct is overrun by the kernel's copy-out.
type Stat_t struct {
	Size    uint64 // 0
	QidPath uint64 // 8
	Atime   uint64 // 16: epoch seconds
	Mtime   uint64 // 24
	Ctime   uint64 // 32
	Mode    uint32 // 40: POSIX mode bits (S_IF* | rwx)
	Nlink   uint32 // 44
	QidVers uint32 // 48
	QidType uint8  // 52
	_       [3]uint8
	Blksize uint32 // 56
	_       uint32 // 60
	Blocks  uint64 // 64
	Uid     uint32 // 72
	Gid     uint32 // 76
	Dev     uint32 // 80: #100 per-instance device number (Plan 9 Chan.dev / st_dev)
	_       uint32 // 84: pad to 88
}

// --- path / name helpers ---

// splitParent splits an absolute or relative path into the parent directory
// (resolvable on its own) and the final component.
func splitParent(path string) (dir, leaf string) {
	for len(path) > 1 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	i := len(path) - 1
	for i >= 0 && path[i] != '/' {
		i--
	}
	switch {
	case i < 0:
		return ".", path
	case i == 0:
		return "/", path[1:]
	default:
		return path[:i], path[i+1:]
	}
}

// openMode opens path with the given kernel omode word against the Territory
// root (the FROM_ROOT sentinel; relative paths resolve against the cwd).
func openMode(path string, omode uintptr) (fd int, err error) {
	p, e := BytePtrFromString(path)
	if e != nil {
		return -1, e
	}
	r1, _, en := Syscall6(SYS_OPEN, walkFromRoot, uintptr(unsafe.Pointer(p)), uintptr(len(path)), omode, 0, 0)
	runtime.KeepAlive(p)
	if en != 0 {
		return -1, en
	}
	return int(r1), nil
}

// --- file ops ---

// Open opens an existing path. mode is the Go open flag; the low bits
// (O_RDONLY/WRONLY/RDWR/EXEC) plus O_TRUNC form the kernel omode.
func Open(path string, mode int) (fd int, err error) {
	return openMode(path, uintptr(mode&omodeMask))
}

// Create makes the final component of path inside its (existing) parent
// directory and returns an opened fd. The parent is opened O_PATH to obtain
// the born-R|W create base SYS_WALK_CREATE requires (RIGHT_WRITE on parent).
func Create(path string, mode int, perm uint32) (fd int, err error) {
	dir, leaf := splitParent(path)
	if leaf == "" || leaf == "." || leaf == ".." {
		return -1, EINVAL
	}
	dfd, e := openMode(dir, SYS_WALK_OPEN_OPATH)
	if e != nil {
		return -1, e
	}
	np, er := BytePtrFromString(leaf)
	if er != nil {
		Close(dfd)
		return -1, er
	}
	omode := uintptr(mode & omodeMask)
	r1, _, en := Syscall6(SYS_WALK_CREATE, uintptr(dfd), uintptr(unsafe.Pointer(np)), uintptr(len(leaf)), omode, uintptr(perm), 0)
	runtime.KeepAlive(np)
	Close(dfd)
	if en != 0 {
		return -1, en
	}
	return int(r1), nil
}

// SYS_RW_MAX mirror -- the kernel's per-call byte-I/O ceiling (128 KiB since
// Thylacine CF-3 A; was 4096). One Read/Write moves up to this in a single
// syscall; the kernel still returns short (the negotiated 9P msize bounds a
// single RPC's payload), and callers loop -- the POSIX contract is unchanged.
const rwMax = 128 * 1024

func Read(fd int, p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	want := len(p)
	if want > rwMax {
		want = rwMax
	}
	r1, _, e := Syscall(SYS_READ, uintptr(fd), uintptr(unsafe.Pointer(&p[0])), uintptr(want))
	runtime.KeepAlive(p)
	if e != 0 {
		return 0, e
	}
	return int(r1), nil
}

func Write(fd int, p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	want := len(p)
	if want > rwMax {
		want = rwMax
	}
	r1, _, e := Syscall(SYS_WRITE, uintptr(fd), uintptr(unsafe.Pointer(&p[0])), uintptr(want))
	runtime.KeepAlive(p)
	if e != 0 {
		return 0, e
	}
	return int(r1), nil
}

// Pread / Pwrite map 1:1 onto SYS_PREAD/SYS_PWRITE (#37): the kernel passes
// the caller's offset straight to the Dev and never reads or advances the fd
// cursor, so concurrent positioned ops on one fd share no mutable state --
// the POSIX contract io.ReaderAt's parallel-use guarantee rides on. (The
// pre-#37 Seek+Read/Write emulation was inherently non-atomic against a
// concurrent cursor move; its cursor-restore bug was #36 layer 2.) Short
// reads/writes are normal (the kernel caps one call at rwMax, and the 9P
// transport clamps a single RPC below that); os.File ReadAt/WriteAt loop.
//
// The len==0 early return (required: &p[0] panics on an empty slice) skips
// the trap, so a zero-length Pread on a non-seekable fd returns (0, nil)
// where the kernel -- and Linux -- would report the ESPIPE-shaped reject.
// Unreachable via os.File (ReadAt/WriteAt never issue empty ops); a known,
// deliberate divergence (#37 audit F2).
func Pread(fd int, p []byte, offset int64) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	want := len(p)
	if want > rwMax {
		want = rwMax
	}
	r1, _, e := Syscall6(SYS_PREAD, uintptr(fd), uintptr(unsafe.Pointer(&p[0])), uintptr(want), uintptr(offset), 0, 0)
	runtime.KeepAlive(p)
	if e != 0 {
		return 0, e
	}
	return int(r1), nil
}

func Pwrite(fd int, p []byte, offset int64) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	want := len(p)
	if want > rwMax {
		want = rwMax
	}
	r1, _, e := Syscall6(SYS_PWRITE, uintptr(fd), uintptr(unsafe.Pointer(&p[0])), uintptr(want), uintptr(offset), 0, 0)
	runtime.KeepAlive(p)
	if e != 0 {
		return 0, e
	}
	return int(r1), nil
}

func Seek(fd int, offset int64, whence int) (off int64, err error) {
	r1, _, e := Syscall(SYS_LSEEK, uintptr(fd), uintptr(offset), uintptr(whence))
	if e != 0 {
		return -1, e
	}
	return int64(r1), nil
}

func Close(fd int) (err error) {
	_, _, e := Syscall(SYS_CLOSE, uintptr(fd), 0, 0)
	if e != 0 {
		return e
	}
	return nil
}

func Fstat(fd int, st *Stat_t) (err error) {
	_, _, e := Syscall(SYS_FSTAT, uintptr(fd), uintptr(unsafe.Pointer(st)), 0)
	if e != 0 {
		return e
	}
	return nil
}

// Stat resolves the path and reads its metadata in ONE syscall (POUNCE
// SYS_STAT = 88): on the disk FS the whole resolution is a single fused
// Twalkgetattr RPC and no handle/Spoor/fid is ever created -- this call is
// the hottest metadata operation of a go build (the pre-POUNCE emulation was
// O_PATH open + Fstat + Close: 3 syscalls, ~13 RPCs on a 4-deep path). The
// X-search authority is identical to the emulation's (path-X only).
func Stat(path string, st *Stat_t) (err error) {
	p, e := BytePtrFromString(path)
	if e != nil {
		return e
	}
	_, _, en := Syscall(SYS_STAT, uintptr(unsafe.Pointer(p)), uintptr(len(path)), uintptr(unsafe.Pointer(st)))
	runtime.KeepAlive(p)
	if en != 0 {
		return en
	}
	return nil
}

// Lstat does not differentiate from Stat at v1.0 (no symlinks; G11).
func Lstat(path string, st *Stat_t) error { return Stat(path, st) }

func Fsync(fd int) (err error) {
	_, _, e := Syscall(SYS_FSYNC, uintptr(fd), 0, 0)
	if e != 0 {
		return e
	}
	return nil
}

// Readdir reads the next run of 9P2000.L dirent records from a directory fd
// into buf (advancing the same cursor SYS_READ / SYS_LSEEK use). Returns the
// byte count (0 == end-of-directory). Each entry: qid(13) + offset(8 LE) +
// type(1) + name_len(2 LE) + name.
// readdirMax mirrors the kernel's SYS_RW_STACK bound: SYS_READDIR REJECTS
// (not clamps) buf_len above 4096, and CF-3 A deliberately kept it there
// when rwMax lifted to 128 KiB -- dirent runs are small and the kernel
// handler stays on its stack scratch. Clamping here keeps any caller-sized
// buffer (os uses 8 KiB blocks) working.
const readdirMax = 4096

func Readdir(fd int, buf []byte) (n int, err error) {
	if len(buf) == 0 {
		return 0, nil
	}
	want := len(buf)
	if want > readdirMax {
		want = readdirMax
	}
	r1, _, e := Syscall(SYS_READDIR, uintptr(fd), uintptr(unsafe.Pointer(&buf[0])), uintptr(want))
	runtime.KeepAlive(buf)
	if e != 0 {
		return 0, e
	}
	return int(r1), nil
}

func Fd2path(fd int) (path string, err error) {
	var buf [512]byte
	r1, _, e := Syscall(SYS_FD2PATH, uintptr(fd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if e != 0 {
		return "", e
	}
	n := int(r1)
	if n < 0 || n > len(buf) {
		n = 0
	}
	return string(buf[:n]), nil
}

func Chdir(path string) (err error) {
	p, e := BytePtrFromString(path)
	if e != nil {
		return e
	}
	_, _, en := Syscall(SYS_CHDIR, uintptr(unsafe.Pointer(p)), uintptr(len(path)), 0)
	runtime.KeepAlive(p)
	if en != 0 {
		return en
	}
	return nil
}

func Fchdir(fd int) (err error) {
	path, e := Fd2path(fd)
	if e != nil {
		return e
	}
	return Chdir(path)
}

func Getwd() (wd string, err error) {
	var buf [512]byte
	r1, _, e := Syscall(SYS_GETCWD, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if e != 0 {
		return "", e
	}
	n := int(r1)
	if n < 0 || n > len(buf) {
		return "", EINVAL
	}
	return string(buf[:n]), nil
}

func Pipe(p []int) (err error) {
	if len(p) != 2 {
		return EINVAL
	}
	r1, r2, e := Syscall(SYS_PIPE, 0, 0, 0)
	if e != 0 {
		return e
	}
	p[0] = int(r1)
	p[1] = int(r2)
	return nil
}

func Dup(fd int) (int, error) {
	r1, _, e := Syscall(SYS_DUP, uintptr(fd), allRights, 0)
	if e != 0 {
		return -1, e
	}
	return int(r1), nil
}

// allRights = RIGHT_READ|RIGHT_WRITE|RIGHT_TRANSFER (kernel handle.h). SYS_DUP
// clamps to the source's rights, so requesting all is the "same rights" dup.
const allRights = 0x7

// Remove and Rename operate on the parent directory by name.
func Remove(path string) error {
	dir, leaf := splitParent(path)
	if leaf == "" || leaf == "." || leaf == ".." {
		return EINVAL
	}
	dfd, e := openMode(dir, SYS_WALK_OPEN_OPATH)
	if e != nil {
		return e
	}
	np, er := BytePtrFromString(leaf)
	if er != nil {
		Close(dfd)
		return er
	}
	_, _, en := Syscall6(SYS_UNLINK, uintptr(dfd), uintptr(unsafe.Pointer(np)), uintptr(len(leaf)), 0, 0, 0)
	runtime.KeepAlive(np)
	Close(dfd)
	if en != 0 {
		return en
	}
	return nil
}

func Rename(oldpath, newpath string) error {
	od, ol := splitParent(oldpath)
	nd, nl := splitParent(newpath)
	ofd, e := openMode(od, SYS_WALK_OPEN_OPATH)
	if e != nil {
		return e
	}
	nfd, e2 := openMode(nd, SYS_WALK_OPEN_OPATH)
	if e2 != nil {
		Close(ofd)
		return e2
	}
	op, er1 := BytePtrFromString(ol)
	npp, er2 := BytePtrFromString(nl)
	if er1 != nil || er2 != nil {
		Close(ofd)
		Close(nfd)
		return EINVAL
	}
	_, _, en := Syscall6(SYS_RENAME, uintptr(ofd), uintptr(unsafe.Pointer(op)), uintptr(len(ol)),
		uintptr(nfd), uintptr(unsafe.Pointer(npp)), uintptr(len(nl)))
	runtime.KeepAlive(op)
	runtime.KeepAlive(npp)
	Close(ofd)
	Close(nfd)
	if en != 0 {
		return en
	}
	return nil
}

const unlinkRemoveDir = 0x200 // SYS_UNLINK_REMOVEDIR

func Mkdir(path string, mode uint32) (err error) {
	dir, leaf := splitParent(path)
	if leaf == "" || leaf == "." || leaf == ".." {
		return EINVAL
	}
	dfd, e := openMode(dir, SYS_WALK_OPEN_OPATH)
	if e != nil {
		return e
	}
	np, er := BytePtrFromString(leaf)
	if er != nil {
		Close(dfd)
		return er
	}
	// DMDIR-folded create; the new dir is opened OREAD by the kernel.
	r1, _, en := Syscall6(SYS_WALK_CREATE, uintptr(dfd), uintptr(unsafe.Pointer(np)), uintptr(len(leaf)),
		omodeOREAD, uintptr(DMDIR|(mode&0o777)), 0)
	runtime.KeepAlive(np)
	Close(dfd)
	if en != 0 {
		return en
	}
	Close(int(r1))
	return nil
}

func Rmdir(path string) error {
	dir, leaf := splitParent(path)
	if leaf == "" || leaf == "." || leaf == ".." {
		return EINVAL
	}
	dfd, e := openMode(dir, SYS_WALK_OPEN_OPATH)
	if e != nil {
		return e
	}
	np, er := BytePtrFromString(leaf)
	if er != nil {
		Close(dfd)
		return er
	}
	_, _, en := Syscall6(SYS_UNLINK, uintptr(dfd), uintptr(unsafe.Pointer(np)), uintptr(len(leaf)), unlinkRemoveDir, 0, 0)
	runtime.KeepAlive(np)
	Close(dfd)
	if en != 0 {
		return en
	}
	return nil
}

// Truncate opens the path for writing and truncates via the fd (POSIX
// truncate(2) requires W permission on the file; the O_WRONLY open enforces
// it -- the open-time perm_check + the omode-derived RIGHT_WRITE the kernel's
// T_WSTAT_SIZE gate demands). Stage 5: cmd/go's module cache rewrites its
// ziphash/lock files through lockedfile, which truncates in place.
func Truncate(path string, length int64) error {
	fd, e := openMode(path, omodeOWRITE)
	if e != nil {
		return e
	}
	err := Ftruncate(fd, length)
	Close(fd)
	return err
}

// Ftruncate = SYS_WSTAT(T_WSTAT_SIZE) -- the 9P Tsetattr size axis (Stage 5;
// shrink discards, extend zero-fills, Stratum-side stm_fs_truncate). The
// fstat no-op fast path is kept deliberately: "truncate to the size the file
// already is" is cmd/go's putIndexEntry defensive call on EVERY cache write,
// and the fstat is served by the guest Larder attr cache (usually no RPC)
// while a wstat is always an RPC that also invalidates the cached attr+pages
// -- the fast path keeps the hot cache-put cheap (#36 layer 3 history: an
// ENOSYS here once deleted every just-written GOCACHE entry).
func Ftruncate(fd int, length int64) error {
	if length < 0 {
		return EINVAL
	}
	var st Stat_t
	if err := Fstat(fd, &st); err == nil && int64(st.Size) == length {
		return nil
	}
	_, _, e := Syscall6(SYS_WSTAT, uintptr(fd), tWstatSize, 0, 0, 0, uintptr(length))
	if e != 0 {
		return e
	}
	return nil
}
func Fchmod(fd int, mode uint32) error { return wstatMode(fd, mode) }
func Chmod(path string, mode uint32) error {
	fd, e := openMode(path, SYS_WALK_OPEN_OPATH)
	if e != nil {
		return e
	}
	er := wstatMode(fd, mode)
	Close(fd)
	return er
}

const (
	tWstatMode = 0x1 // T_WSTAT_MODE (kernel/include/thylacine/syscall.h)
	tWstatSize = 0x8 // T_WSTAT_SIZE (== P9_SETATTR_SIZE; ftruncate, Stage 5)
)

func wstatMode(fd int, mode uint32) error {
	_, _, e := Syscall6(SYS_WSTAT, uintptr(fd), tWstatMode, uintptr(mode&0o777), 0, 0, 0)
	if e != 0 {
		return e
	}
	return nil
}

// --- identity ---

func Getpid() int  { r, _, _ := Syscall(SYS_GETPID, 0, 0, 0); return int(r) }
func Getppid() int { return 0 } // no syscall; parent pid not exposed at v1.0
func Getuid() int  { r, _, _ := Syscall(SYS_GETUID, 0, 0, 0); return int(r) }
func Getgid() int  { r, _, _ := Syscall(SYS_GETGID, 0, 0, 0); return int(r) }
func Geteuid() int { return Getuid() }
func Getegid() int { return Getgid() }

func Getgroups() (gids []int, err error) {
	return []int{Getgid()}, nil
}

// Exit is provided by the runtime (package syscall.go). Getpagesize too.

// Clock for time.Now fast paths that route through syscall (most go through
// runtime). Provided for completeness.
func Gettimeofday(tv *Timeval) error {
	var ts Timespec
	_, _, e := Syscall(SYS_CLOCK_GETTIME, 0 /*REALTIME*/, uintptr(unsafe.Pointer(&ts)), 0)
	if e != 0 {
		return e
	}
	*tv = NsecToTimeval(ts.Sec*1e9 + ts.Nsec)
	return nil
}
