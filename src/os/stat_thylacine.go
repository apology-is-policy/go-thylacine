// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package os

import (
	"syscall"
	"time"
)

// fileInfoFromStat builds a FileInfo from a Thylacine t_stat record. name is
// the path the file was reached by; only its final component is kept.
func fileInfoFromStat(st *syscall.Stat_t, name string) *fileStat {
	fs := &fileStat{
		name:    basename(name),
		size:    int64(st.Size),
		modTime: time.Unix(int64(st.Mtime), 0),
		sys:     *st,
	}
	fs.mode = FileMode(st.Mode & 0o777)
	switch st.Mode & syscall.S_IFMT {
	case syscall.S_IFDIR:
		fs.mode |= ModeDir
	case syscall.S_IFCHR:
		fs.mode |= ModeCharDevice | ModeDevice
	case syscall.S_IFBLK:
		fs.mode |= ModeDevice
	case syscall.S_IFIFO:
		fs.mode |= ModeNamedPipe
	case syscall.S_IFLNK:
		fs.mode |= ModeSymlink
	case syscall.S_IFSOCK:
		fs.mode |= ModeSocket
	}
	// Some Devs (e.g. devramfs directories) carry the directory bit only in
	// the 9P qid type (QTDIR = 0x80).
	if st.QidType&0x80 != 0 {
		fs.mode |= ModeDir
	}
	if st.Mode&syscall.S_ISUID != 0 {
		fs.mode |= ModeSetuid
	}
	if st.Mode&syscall.S_ISGID != 0 {
		fs.mode |= ModeSetgid
	}
	if st.Mode&syscall.S_ISVTX != 0 {
		fs.mode |= ModeSticky
	}
	return fs
}

// basename returns the final path component of name.
func basename(name string) string {
	i := len(name) - 1
	for i > 0 && name[i] == '/' {
		name = name[:i]
		i--
	}
	for i >= 0 && name[i] != '/' {
		i--
	}
	return name[i+1:]
}

func statNolog(name string) (FileInfo, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(name, &st); err != nil {
		return nil, &PathError{Op: "stat", Path: name, Err: err}
	}
	return fileInfoFromStat(&st, name), nil
}

func lstatNolog(name string) (FileInfo, error) {
	return statNolog(name)
}

// For testing.
func atime(fi FileInfo) time.Time {
	st := fi.Sys().(*syscall.Stat_t)
	return time.Unix(int64(st.Atime), 0)
}
