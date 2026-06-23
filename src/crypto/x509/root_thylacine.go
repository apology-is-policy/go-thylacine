// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build thylacine

package x509

import (
	"os"
)

// Thylacine has no OS certificate-verification API, so x509 verifies against a
// PEM root bundle read here. The bundle path is reconciled with the system CA
// bundle when TLS-for-`go get` lands (Stage 5); SSL_CERT_FILE overrides, per the
// unix convention. Absent any bundle, an empty (non-nil) pool is returned rather
// than an error: a local `go build` performs no certificate verification, so it
// is unaffected, while TLS verification against an empty pool fails closed.
var certFiles = []string{
	"/lib/tls/ca-bundle.pem",
	"/etc/ssl/cert.pem",
}

func (c *Certificate) systemVerify(opts *VerifyOptions) (chains [][]*Certificate, err error) {
	return nil, nil
}

func loadSystemRoots() (*CertPool, error) {
	roots := NewCertPool()
	files := certFiles
	if f := os.Getenv("SSL_CERT_FILE"); f != "" {
		files = []string{f}
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err == nil {
			roots.AppendCertsFromPEM(data)
			return roots, nil
		}
	}
	return roots, nil
}
