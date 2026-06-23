// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package syscall

// Thylacine passes no environment to a Proc at v1.0 (the kernel
// SYS_SPAWN_FULL_ARGV carries argv but not envp -- the envp slot is reserved,
// G15). The environment is therefore empty and immutable from the program's
// point of view; Setenv/Unsetenv succeed but reach no child.

func Getenv(key string) (value string, found bool) { return "", false }
func Setenv(key, value string) error               { return nil }
func Unsetenv(key string) error                    { return nil }
func Clearenv()                                    {}
func Environ() []string                            { return nil }
