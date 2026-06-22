// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// Thylacine memory model (the third hard divergence from the Linux base).
//
// Thylacine has no mmap. The native primitive is SYS_BURROW_ATTACH(length),
// which allocates an anonymous, eagerly-committed, RW (W^X-safe) region at a
// KERNEL-CHOSEN virtual address and returns that address. There is no
// PROT_NONE reservation, no MAP_FIXED (the caller cannot demand an address),
// and no madvise. SYS_BURROW_DETACH(v, n) frees a region.
//
// The Go allocator's reserve-then-map-at-a-fixed-address model therefore does
// not map onto Thylacine directly. The Stage-1 strategy is:
//
//   - sysReserve commits immediately and returns the kernel-chosen address.
//     Callers already use the returned address (the hint is advisory on every
//     OS), so this is correct -- only wasteful, because a reservation is
//     backed by physical pages up front.
//   - sysMap is then a no-op: [v, v+n) is already committed at the address
//     sysReserve handed back.
//   - sysUnused / sysUsed / sysFault are no-ops; there is no decommit, so
//     pages stay resident until sysFree.
//
// The proper fix -- a BURROW_ATTACH(LAZY) flag that demand-allocates pages on
// fault (composing with the REVENANT fault arm) -- is the arc's one
// audit-bearing kernel change and lands separately.

//go:noescape
func sysBurrowAttach(n uintptr) uintptr

//go:noescape
func sysBurrowDetach(v unsafe.Pointer, n uintptr) int32

// burrowAttach returns the committed base address, or nil on failure. A
// successful return is a small positive user VA; an errno is negative.
//
//go:nosplit
func burrowAttach(n uintptr) unsafe.Pointer {
	p := sysBurrowAttach(n)
	if int64(p) < 0 {
		return nil
	}
	return unsafe.Pointer(p)
}

//go:nosplit
func sysAllocOS(n uintptr, vmaName string) unsafe.Pointer {
	return burrowAttach(n)
}

func sysUnusedOS(v unsafe.Pointer, n uintptr) {}

func sysUsedOS(v unsafe.Pointer, n uintptr) {}

func sysHugePageOS(v unsafe.Pointer, n uintptr) {}

func sysNoHugePageOS(v unsafe.Pointer, n uintptr) {}

func sysHugePageCollapseOS(v unsafe.Pointer, n uintptr) {}

//go:nosplit
func sysFreeOS(v unsafe.Pointer, n uintptr) {
	sysBurrowDetach(v, n)
}

func sysFaultOS(v unsafe.Pointer, n uintptr) {
	// No decommit primitive; leave the pages committed. (A future
	// BURROW_ATTACH(LAZY) makes this a real fault-in-on-next-touch.)
}

func sysReserveOS(v unsafe.Pointer, n uintptr, vmaName string) unsafe.Pointer {
	// Reserve == commit on Thylacine; the hint v is ignored and the
	// kernel-chosen address is returned. The caller uses the returned value.
	return burrowAttach(n)
}

func sysMapOS(v unsafe.Pointer, n uintptr, vmaName string) {
	// No-op: sysReserveOS already committed [v, v+n) at v.
}
