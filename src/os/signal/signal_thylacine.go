// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signal

import (
	"os"
	"syscall"
)

// Thylacine signals are Plan 9-style notes (syscall.Note), but the Go runtime
// does not yet deliver them into os/signal: the Stage-0b GOOS=thylacine runtime
// stubbed the signal_recv/signal_enable machinery the plan9 port relies on. So
// signal.Notify registration is accepted but never fires -- an honest no-op.
// This mirrors signal_plan9.go's surface minus the runtime delivery loop
// (watchSignalLoop is left unset, so no signal_recv is ever called). Wiring
// notes into os/signal is a later enhancement, not a toolchain prerequisite.

var sigtab = make(map[os.Signal]int)

const numSig = 256

func signum(sig os.Signal) int {
	switch sig := sig.(type) {
	case syscall.Note:
		n, ok := sigtab[sig]
		if !ok {
			n = len(sigtab) + 1
			if n > numSig {
				return -1
			}
			sigtab[sig] = n
		}
		return n
	default:
		return -1
	}
}

func enableSignal(sig int)       {}
func disableSignal(sig int)      {}
func ignoreSignal(sig int)       {}
func signalIgnored(sig int) bool { return false }
