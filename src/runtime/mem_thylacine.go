// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// Thylacine memory model (the third hard divergence from the Linux base).
//
// Thylacine has no mmap. The native primitives are:
//
//   - SYS_BURROW_ATTACH_LAZY(length): reserve an anonymous, demand-zero, RW
//     (W^X-safe) region at a KERNEL-CHOSEN virtual address and return it. No
//     physical pages are committed until a page is first touched, at which
//     point the kernel zero-fills and installs it (the Linux overcommit
//     contract). There is no MAP_FIXED -- the caller cannot demand an address.
//   - SYS_BURROW_DECOMMIT(v, n): drop the resident pages of [v, v+n) (the
//     madvise(MADV_DONTNEED) analog). The reservation stays; a later touch
//     re-faults a fresh zero page.
//   - SYS_BURROW_DETACH(v, n): free a region entirely.
//
// The Go allocator's reserve-then-map-at-a-fixed-address model maps cleanly
// onto this:
//
//   - sysReserve reserves lazily and returns the kernel-chosen address.
//     Callers already use the returned address (the hint is advisory on every
//     OS), and nothing is committed up front -- a reservation is free.
//   - sysMap is a no-op: [v, v+n) is already usable at the address sysReserve
//     handed back; pages commit on first touch.
//   - sysUnused / sysFault decommit -- the scavenger's freed pages return to
//     the kernel and RSS shrinks. sysUsed is a no-op (a touch re-faults).
//   - sysFree detaches the whole region.

//go:noescape
func sysBurrowAttachLazy(n uintptr) uintptr

//go:noescape
func sysBurrowDetach(v unsafe.Pointer, n uintptr) int32

//go:noescape
func sysBurrowDecommit(v unsafe.Pointer, n uintptr) int32

// burrowAttachLazy reserves an n-byte demand-zero region and returns its base
// address, or nil on failure. A successful return is a small positive user VA;
// an errno is negative.
//
//go:nosplit
func burrowAttachLazy(n uintptr) unsafe.Pointer {
	p := sysBurrowAttachLazy(n)
	if int64(p) < 0 {
		return nil
	}
	return unsafe.Pointer(p)
}

//go:nosplit
func sysAllocOS(n uintptr, vmaName string) unsafe.Pointer {
	return burrowAttachLazy(n)
}

func sysUnusedOS(v unsafe.Pointer, n uintptr) {
	// Decommit: return the pages to the kernel. The reservation stays, so a
	// later sysUsed + touch re-faults a fresh zero page.
	sysBurrowDecommit(v, n)
}

func sysUsedOS(v unsafe.Pointer, n uintptr) {
	// No-op: the region is still reserved and demand-zero. The next write
	// faults a page in (zeroed), which is what sysUsed needs.
}

func sysHugePageOS(v unsafe.Pointer, n uintptr) {}

func sysNoHugePageOS(v unsafe.Pointer, n uintptr) {}

func sysHugePageCollapseOS(v unsafe.Pointer, n uintptr) {}

//go:nosplit
func sysFreeOS(v unsafe.Pointer, n uintptr) {
	sysBurrowDetach(v, n)
}

func sysFaultOS(v unsafe.Pointer, n uintptr) {
	// No PROT_NONE-over-a-reservation primitive, so an access cannot be made
	// to trap. Decommit is the closest available behavior: drop the pages so a
	// stale access reads a fresh zero page rather than old data, and RSS
	// shrinks. (sysFault is a debugging/reclaim helper, not a hot path.)
	sysBurrowDecommit(v, n)
}

func sysReserveOS(v unsafe.Pointer, n uintptr, vmaName string) unsafe.Pointer {
	// Reserve lazily; the hint v is ignored and the kernel-chosen address is
	// returned. The caller uses the returned value. Nothing commits until a
	// page is touched.
	return burrowAttachLazy(n)
}

func sysMapOS(v unsafe.Pointer, n uintptr, vmaName string) {
	// No-op: sysReserveOS already reserved [v, v+n) at v as demand-zero.
}
