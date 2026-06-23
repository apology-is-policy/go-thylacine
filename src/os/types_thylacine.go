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
	// Thylacine carries no Dev id in t_stat; the 9P qid (path+type) is the
	// per-Dev identity. Cross-Dev collisions are possible but rare at v1.0.
	return fs1.sys.QidPath == fs2.sys.QidPath && fs1.sys.QidType == fs2.sys.QidType
}
