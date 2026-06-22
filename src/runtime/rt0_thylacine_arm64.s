// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

// _rt0_arm64_thylacine is the kernel-visible entry point. The Thylacine
// loader hands control here with argc at 0(RSP) and argv at 8(RSP), the same
// SysV-ish stack the Linux arm64 _rt0 expects. There is no c-shared / library
// entry on Thylacine.
TEXT _rt0_arm64_thylacine(SB),NOSPLIT|NOFRAME,$0
	MOVD	0(RSP), R0	// argc
	ADD	$8, RSP, R1	// argv
	BL	main(SB)

TEXT main(SB),NOSPLIT|NOFRAME,$0
	MOVD	$runtime·rt0_go(SB), R2
	BL	(R2)
exit:
	MOVD	$0, R0
	MOVD	$60, R8		// SYS_exit_group
	SVC
	B	exit
