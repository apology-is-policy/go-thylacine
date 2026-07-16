// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package user

import "errors"

// Thylacine has no /etc/passwd or /etc/group: identity is the kernel principal
// (corvus / principal_id, surfaced through getuid/getgid). Current() works --
// its Uid, Gid, Username ($USER), and HomeDir ($HOME) come from lookup_stubs.go's
// currentUID/currentGID plus the environment. Name/id Lookup is unsupported, as
// on Plan 9 (lookup_plan9.go returns syscall.EPLAN9).

var errUserLookupUnsupported = errors.New("user: Lookup not implemented on thylacine")

func lookupUser(string) (*User, error)     { return nil, errUserLookupUnsupported }
func lookupUserId(string) (*User, error)   { return nil, errUserLookupUnsupported }
func lookupGroup(string) (*Group, error)   { return nil, errUserLookupUnsupported }
func lookupGroupId(string) (*Group, error) { return nil, errUserLookupUnsupported }
func listGroups(*User) ([]string, error)   { return nil, errUserLookupUnsupported }
