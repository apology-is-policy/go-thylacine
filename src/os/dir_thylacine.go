// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package os

import (
	"io"
	"io/fs"
	"syscall"
)

// dtDir is the 9P2000.L directory d_type carried in each SYS_READDIR record.
const dtDir = 4

// readdir reads directory entries via SYS_READDIR, parsing the 9P2000.L dirent
// stream: per record qid(13) + offset(8 LE) + type(1) + name_len(2 LE) + name.
func (file *File) readdir(n int, mode readdirMode) (names []string, dirents []DirEntry, infos []FileInfo, err error) {
	var d *dirInfo
	for {
		d = file.dirinfo.Load()
		if d != nil {
			break
		}
		newD := new(dirInfo)
		if file.dirinfo.CompareAndSwap(nil, newD) {
			d = newD
			break
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if n <= 0 {
		n = -1
	}
	for n != 0 {
		if d.bufp >= d.nbuf {
			nb, e := syscall.Readdir(file.sysfd, d.buf[:])
			d.bufp, d.nbuf = 0, nb
			if e != nil {
				return names, dirents, infos, &PathError{Op: "readdir", Path: file.name, Err: e}
			}
			if nb == 0 {
				break // end of directory
			}
		}

		b := d.buf[d.bufp:d.nbuf]
		// A full record is at least the fixed 24-byte prefix; the kernel
		// returns only complete records, so a short tail means we are done.
		if len(b) < 24 {
			break
		}
		dtype := b[21]
		namelen := int(b[22]) | int(b[23])<<8
		rec := 24 + namelen
		if len(b) < rec {
			break
		}
		name := string(b[24:rec])
		d.bufp += rec

		if name == "." || name == ".." {
			continue
		}

		switch mode {
		case readdirName:
			names = append(names, name)
		case readdirDirEntry:
			typ := FileMode(0)
			if dtype == dtDir {
				typ = ModeDir
			}
			dirents = append(dirents, &dirEntry{parent: file.name, name: name, typ: typ})
		default: // readdirInfo
			fi, e := lstatNolog(file.name + "/" + name)
			if e != nil {
				return names, dirents, infos, e
			}
			infos = append(infos, fi)
		}
		n--
	}

	if n > 0 && len(names)+len(dirents)+len(infos) == 0 {
		return nil, nil, nil, io.EOF
	}
	return names, dirents, infos, nil
}

type dirEntry struct {
	parent string
	name   string
	typ    FileMode
}

func (de *dirEntry) Name() string            { return de.name }
func (de *dirEntry) IsDir() bool             { return de.typ.IsDir() }
func (de *dirEntry) Type() FileMode          { return de.typ }
func (de *dirEntry) Info() (FileInfo, error) { return lstatNolog(de.parent + "/" + de.name) }

func (de *dirEntry) String() string {
	return fs.FormatDirEntry(de)
}
