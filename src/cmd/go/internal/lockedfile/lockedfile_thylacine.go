// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package lockedfile

import (
	"io/fs"
	"os"
)

// Thylacine's filelock is a best-effort no-op (see
// internal/filelock/filelock_thylacine.go), so the generic variant's
// strip-O_TRUNC-then-f.Truncate(0)-after-locking ordering has no motivation
// here — and it breaks: the kernel's only truncate surface at v1.0 is
// O_TRUNC-at-open (syscall.Ftruncate honors just the size==length no-op; a
// real shrink is honest ENOSYS until the T_WSTAT_SIZE lift, #365). Passing
// O_TRUNC straight through gives real kernel truncation with semantics
// unchanged under no-op locks. Without this, the first cache trim after
// trim.txt ages past 24h fails every `go build` with
// "truncate .../trim.txt: function not implemented" (#364).
func openFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func closeFile(f *os.File) error {
	return f.Close()
}
