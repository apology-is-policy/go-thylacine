// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package exec

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound is the error resulting if a path search failed to find an executable file.
var ErrNotFound = errors.New("executable file not found in $PATH")

// findExecutable reports whether file is a runnable program. Thylacine gates
// execution on namespace X-search + an OEXEC open (the kernel's
// exec_load_from_namespace), NOT on the POSIX 0111 mode bits -- a baked
// binary may carry any mode -- so the check is "exists and is not a
// directory," not "has an execute bit."
func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() {
		return nil
	}
	return fs.ErrPermission
}

// LookPath searches for an executable named file. If file begins with "/",
// "#", "./", or "../", it is tried directly. Otherwise the directories in the
// POSIX "PATH" environment variable are consulted -- the per-Proc /env (G15),
// which the session seeds (login: PATH=/bin:/goroot/bin) and goenvs reads at
// startup. The variable is UPPERCASE "PATH" (Thylacine's POSIX-shaped /env),
// not Plan 9's lowercase "$path": it must match what the session seeds, or a
// Go program's exec.LookPath (e.g. gopls resolving "go") finds nothing while
// os.Getenv("PATH") is non-empty.
//
// On success the result is an absolute path.
func LookPath(file string) (string, error) {
	if err := validateLookPath(filepath.Clean(file)); err != nil {
		return "", &Error{file, err}
	}

	// skip the path lookup for these prefixes
	skip := []string{"/", "#", "./", "../"}

	for _, p := range skip {
		if strings.HasPrefix(file, p) {
			err := findExecutable(file)
			if err == nil {
				return file, nil
			}
			return "", &Error{file, err}
		}
	}

	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		path := filepath.Join(dir, file)
		if err := findExecutable(path); err == nil {
			if !filepath.IsAbs(path) {
				if execerrdot.Value() != "0" {
					return path, &Error{file, ErrDot}
				}
				execerrdot.IncNonDefault()
			}
			return path, nil
		}
	}
	return "", &Error{file, ErrNotFound}
}

// lookExtensions is a no-op on non-Windows platforms, since they do not
// restrict executables to specific extensions.
func lookExtensions(path, dir string) (string, error) {
	return path, nil
}
