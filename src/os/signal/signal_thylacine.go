// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signal

import (
	"os"
	"sync"
	"syscall"
)

// Thylacine signals are Plan 9-style notes (syscall.Note). The kernel does not
// yet deliver them into the runtime (the notes bridge will runtime.sigsend
// them), but the runtime's sigqueue machinery (sigqueue.go, !plan9) is fully
// compiled in, and signal.Stop -> runtime.signalWaitUntilIdle spins Gosched()
// until the receiver loop parks in signal_recv (sig.state == sigReceiving).
// Only the loop goroutine below ever sets that state, so it must run even
// while no note can arrive: a prior version left watchSignalLoop unset as an
// "honest no-op", which turned every signal.Stop into an unbounded Gosched
// storm (cmd/go wraps each `go tool X` run in Notify/Stop; the Stop livelocked
// the go driver at ~5M yields/s forever -- Thylacine #358). The parked loop
// goroutine costs one waiting G and nothing else.

// Defined by the runtime package (sigqueue.go).
func signal_disable(uint32)
func signal_enable(uint32)
func signal_ignore(uint32)
func signal_ignored(uint32) bool
func signal_recv() uint32

func loop() {
	for {
		process(noteOf(signal_recv()))
	}
}

func init() {
	watchSignalLoop = loop
}

// Notes are strings, so signal numbers are assigned at first registration.
// The reverse table lets the receiver loop map a runtime-delivered number
// back to the registered Note; the notes bridge will inject numbers via
// runtime.sigsend under this same assignment. sigMu is a leaf lock (signum
// is called under handlers.Lock, noteOf from the receiver loop without it).
var (
	sigMu   sync.Mutex
	sigtab  = make(map[os.Signal]int)
	sigrtab = make(map[int]os.Signal)
)

// numSig matches the runtime's _NSIG: signal_recv scans numbers < 65, so a
// larger assignment here would register Notes the runtime can never deliver.
const numSig = 65

func signum(sig os.Signal) int {
	switch sig := sig.(type) {
	case syscall.Note:
		sigMu.Lock()
		defer sigMu.Unlock()
		n, ok := sigtab[sig]
		if !ok {
			n = len(sigtab) + 1
			if n >= numSig {
				return -1
			}
			sigtab[sig] = n
			sigrtab[n] = sig
		}
		return n
	default:
		return -1
	}
}

func noteOf(n uint32) os.Signal {
	sigMu.Lock()
	defer sigMu.Unlock()
	if s, ok := sigrtab[int(n)]; ok {
		return s
	}
	return syscall.Note("")
}

func enableSignal(sig int)       { signal_enable(uint32(sig)) }
func disableSignal(sig int)      { signal_disable(uint32(sig)) }
func ignoreSignal(sig int)       { signal_ignore(uint32(sig)) }
func signalIgnored(sig int) bool { return signal_ignored(uint32(sig)) }
