// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package os

import (
	"syscall"
	"time"
)

// A fileStat is the implementation of FileInfo returned by Stat and Lstat.
type fileStat struct {
	name    string
	size    int64
	mode    FileMode
	modTime time.Time
	sys     syscall.Stat_t
}

func (fs *fileStat) Size() int64        { return fs.size }
func (fs *fileStat) Mode() FileMode     { return fs.mode }
func (fs *fileStat) ModTime() time.Time { return fs.modTime }
func (fs *fileStat) Sys() any           { return &fs.sys }

func sameFile(fs1, fs2 *fileStat) bool {
	// #100: t_stat now carries Dev (the per-mount/session identity, Plan 9
	// Chan.dev). Two names are the same file iff they share device AND the 9P
	// qid (path+type) -- Dev disambiguates a qid.path reused across datasets.
	return fs1.sys.Dev == fs2.sys.Dev &&
		fs1.sys.QidPath == fs2.sys.QidPath && fs1.sys.QidType == fs2.sys.QidType
}
