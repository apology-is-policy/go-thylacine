// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package poll

import (
	"sync"
	"syscall"
)

// asyncIO runs one interruptible I/O system call in a locked OS thread so
// that, in a later stage, a deadline timer can cancel it via a note. At v1.0
// Cancel is a no-op: blocking I/O completes normally; deadlines do not abort
// an in-flight call (Stage 3c wires the interrupt via SYS_POSTNOTE).
type asyncIO struct {
	res chan result
	mu  sync.Mutex
	pid int
}

type result struct {
	n   int
	err error
}

func newAsyncIO(fn func([]byte) (int, error), b []byte) *asyncIO {
	aio := &asyncIO{res: make(chan result, 0)}
	aio.mu.Lock()
	go func() {
		aio.pid = syscall.Getpid()
		aio.mu.Unlock()

		n, err := fn(b)

		aio.mu.Lock()
		aio.pid = -1
		aio.mu.Unlock()

		aio.res <- result{n, err}
	}()
	return aio
}

// Cancel would interrupt the I/O operation. Not yet implemented on Thylacine.
func (aio *asyncIO) Cancel() {}

func (aio *asyncIO) Wait() (int, error) {
	res := <-aio.res
	return res.n, res.err
}
