// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package filelock

// Thylacine does not yet expose kernel advisory file locks, so locking is a
// best-effort no-op: behave like a filesystem that does not enforce
// exclusion rather than failing every lockedfile consumer. This mirrors the
// plan9 degradation when a file server does not honor DMEXCL. Within one go
// invocation the cache and module-fetch paths already serialize in process;
// cross-invocation uses (cache trim, env writes) tolerate unenforced
// exclusion. Replace with real locks when the kernel grows them.

type lockType int8

const (
	readLock = iota + 1
	writeLock
)

func lock(f File, lt lockType) error {
	return nil
}

func unlock(f File) error {
	return nil
}
