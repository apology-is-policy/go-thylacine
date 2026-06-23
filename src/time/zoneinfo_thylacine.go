// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package time

// Thylacine ships no timezone database at v1.0, so there are no platform zone
// sources and the local zone is UTC. (A /lib/zoneinfo source is a later add.)

var platformZoneSources []string

func initLocal() {
	localLoc.name = "UTC"
}
