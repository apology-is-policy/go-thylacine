// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

// Environment variables for GOOS=thylacine (G15). The environment comes from
// the per-Proc /env device (ARCH 9.7): the runtime's goenvs reads /env into the
// runtime `envs` slice at startup (a child inherits a copy via env_clone_into).
// Getenv/Environ read that cache; Setenv/Unsetenv mutate it in-process (POSIX
// semantics) WITHOUT writing back to /env -- exactly as plan9 does, so a child
// does not observe a parent's post-spawn Setenv. (To change the on-device
// environment, write /env/KEY -- os.WriteFile.) This mirrors env_unix.go minus
// the cgo C-environ sync (CGO_ENABLED=0, so there is no C environ to keep
// coherent).

package syscall

import "sync"

var (
	// envLock guards env and envs.
	envLock sync.RWMutex

	// env maps from an environment variable to its first occurrence in envs.
	env map[string]int

	// envs is provided by the runtime (populated by goenvs from /env). Elements
	// are "key=value"; an empty string means deleted (or an ignored duplicate).
	envs []string = runtime_envs()
)

func runtime_envs() []string // in package runtime

var copyenv = sync.OnceFunc(func() {
	env = make(map[string]int)
	for i, s := range envs {
		for j := 0; j < len(s); j++ {
			if s[j] == '=' {
				key := s[:j]
				if _, ok := env[key]; !ok {
					env[key] = i // first mention of key
				} else {
					// Clear duplicate keys so Unsetenv can delete the first
					// without unshadowing a later one (a security concern).
					envs[i] = ""
				}
				break
			}
		}
	}
})

func Getenv(key string) (value string, found bool) {
	copyenv()
	if len(key) == 0 {
		return "", false
	}

	envLock.RLock()
	defer envLock.RUnlock()

	i, ok := env[key]
	if !ok {
		return "", false
	}
	s := envs[i]
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[i+1:], true
		}
	}
	return "", false
}

func Setenv(key, value string) error {
	copyenv()
	if len(key) == 0 {
		return EINVAL
	}
	for i := 0; i < len(key); i++ {
		if key[i] == '=' || key[i] == 0 {
			return EINVAL
		}
	}
	for i := 0; i < len(value); i++ {
		if value[i] == 0 {
			return EINVAL
		}
	}

	envLock.Lock()
	defer envLock.Unlock()

	i, ok := env[key]
	kv := key + "=" + value
	if ok {
		envs[i] = kv
	} else {
		i = len(envs)
		envs = append(envs, kv)
	}
	env[key] = i
	return nil
}

func Unsetenv(key string) error {
	copyenv()

	envLock.Lock()
	defer envLock.Unlock()

	if i, ok := env[key]; ok {
		envs[i] = ""
		delete(env, key)
	}
	return nil
}

func Clearenv() {
	copyenv()

	envLock.Lock()
	defer envLock.Unlock()

	env = make(map[string]int)
	envs = []string{}
}

func Environ() []string {
	copyenv()
	envLock.RLock()
	defer envLock.RUnlock()
	a := make([]string, 0, len(envs))
	for _, e := range envs {
		if e != "" {
			a = append(a, e)
		}
	}
	return a
}
