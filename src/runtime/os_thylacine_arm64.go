// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

func osArchInit() {}

//go:nosplit
func cputicks() int64 {
	// nanotime() is a sufficient approximation of CPU ticks for the profiler;
	// Thylacine exposes no cycle counter to EL0 at this stage.
	return nanotime()
}
