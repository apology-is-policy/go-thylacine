// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package os

// hostname returns the system hostname. Thylacine exposes no sysname source at
// v1.0, so this is a fixed identity.
func hostname() (name string, err error) {
	return "thylacine", nil
}
