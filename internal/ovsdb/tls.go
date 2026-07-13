package ovsdb

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// BuildTLSConfig builds a *tls.Config for ssl: OVSDB connections (OVN
// Northbound/Southbound and the per-chassis OVS management pool) from the given
// client certificate, key and CA bundle paths.
//
// It is all-or-none: passing all three empty returns (nil, nil), which leaves
// the caller dialing plain tcp: endpoints (and ssl: endpoints fall back to Go's
// default verification with no client certificate). Passing only a subset is an
// error rather than a silently weaker configuration. When all three are set,
// the certificate/key pair is loaded into Certificates, the CA bundle into
// RootCAs, and the minimum version pinned to TLS 1.2.
func BuildTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	if certFile == "" && keyFile == "" && caFile == "" {
		return nil, nil
	}
	if certFile == "" || keyFile == "" || caFile == "" {
		return nil, fmt.Errorf("TLS requires cert, key and CA together (got cert=%q key=%q ca=%q)", certFile, keyFile, caFile)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading client cert/key: %w", err)
	}

	caPEM, err := os.ReadFile(caFile) // #nosec G304 -- operator-supplied CA bundle path (flag/env), not attacker input
	if err != nil {
		return nil, fmt.Errorf("reading CA bundle: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("CA bundle %s contains no valid certificates", caFile)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}
