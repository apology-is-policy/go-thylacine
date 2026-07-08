// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build arm64 && thylacine

package cpu

func osInit() {
	// The Thylacine kernel publishes a Linux-compatible AT_HWCAP word in
	// the exec auxv (derived from ID_AA64ISAR0/PFR0; the runtime's sysargs
	// stores it in HWCap before osInit runs). The word never carries
	// hwcap_CPUID: Thylacine does not trap-and-emulate EL0 MRS of
	// MIDR_EL1, so hwcapInit's Neoverse probe must stay unreachable.
	hwcapInit("thylacine")
}
