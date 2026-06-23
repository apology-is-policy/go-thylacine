// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exec

import (
	"io/fs"
	"syscall"
)

// skipStdinCopyError reports whether the provided stdin-copy error should be
// ignored. On Thylacine a write to a pipe whose read end has closed returns
// EPIPE; if the child exited (closing its stdin) before we finished feeding
// it, that is benign -- the program completed otherwise. (Issue 35753.)
func skipStdinCopyError(err error) bool {
	pe, ok := err.(*fs.PathError)
	return ok && pe.Op == "write" && pe.Err == syscall.EPIPE
}
