// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package syscall

// Open / create flags. The low two bits are the Plan 9 omode the kernel's
// SYS_OPEN / SYS_WALK_CREATE consume directly (OREAD / OWRITE / ORDWR /
// OEXEC). The higher bits are Go-level open flags interpreted by the os
// package's openFileNolog (which routes O_CREATE through SYS_WALK_CREATE
// and folds O_TRUNC into the kernel omode).
const (
	O_RDONLY = 0x0
	O_WRONLY = 0x1
	O_RDWR   = 0x2
	O_EXEC   = 0x3 // Plan 9 OEXEC

	O_TRUNC = 0x10 // kernel OTRUNC (folds into the omode word)

	O_CREAT    = 0x100 // the os package spells this O_CREATE = syscall.O_CREAT
	O_EXCL     = 0x200
	O_APPEND   = 0x400
	O_CLOEXEC  = 0x800
	O_SYNC     = 0x1000
	O_NONBLOCK = 0x2000
)

// Kernel omode bits (kernel/include/thylacine/syscall.h).
const (
	omodeOREAD  = 0
	omodeOWRITE = 1
	omodeORDWR  = 2
	omodeOEXEC  = 3
	omodeOTRUNC = 0x10
	omodeMask   = 0x13 // OREAD|OWRITE|ORDWR|OEXEC + OTRUNC -- the valid set
)

// SYS_WALK_OPEN_OPATH (0x80): a walk-only navigation handle, born R|W with
// no TRANSFER. It is the create base SYS_WALK_CREATE requires (RIGHT_WRITE on
// the parent dir) and the handle Stat fstats. kernel/include/thylacine/syscall.h.
const SYS_WALK_OPEN_OPATH = 0x80

// SYS_LSEEK whence (T_SEEK_*).
const (
	SEEK_SET = 0
	SEEK_CUR = 1
	SEEK_END = 2
)

// POSIX file-type and permission bits carried in struct t_stat.mode.
const (
	S_IFMT   = 0xf000
	S_IFIFO  = 0x1000
	S_IFCHR  = 0x2000
	S_IFDIR  = 0x4000
	S_IFBLK  = 0x6000
	S_IFREG  = 0x8000
	S_IFLNK  = 0xa000
	S_IFSOCK = 0xc000

	S_ISUID = 0x800
	S_ISGID = 0x400
	S_ISVTX = 0x200

	S_IRUSR = 0x100
	S_IWUSR = 0x080
	S_IXUSR = 0x040
	S_IRWXU = 0x1c0
)

// Plan 9 directory bit for a SYS_WALK_CREATE of a directory (DMDIR).
const DMDIR = 0x80000000

// SYS_SPAWN_FULL_ARGV bounds (kernel/include/thylacine/syscall.h).
const (
	spawnNameMax     = 64
	spawnArgvMax     = 16
	spawnArgvDataMax = 4096
	spawnMaxFds      = 16
)
