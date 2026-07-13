package ovsdb

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestCert generates a self-signed certificate for 127.0.0.1 and writes the
// certificate and key to dir, returning their paths. The same certificate serves
// as its own CA bundle, so it can be handed to BuildTLSConfig as cert, key and
// CA alike.
func writeTestCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "northwatch-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "cert.pem")
	require.NoError(t, os.WriteFile(certPath,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPath = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(keyPath,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))

	return certPath, keyPath
}

// serveTLSFront puts a TLS listener in front of an existing "unix:<path>" OVSDB
// address, so an in-memory test server can be reached over ssl:. It returns the
// ssl: address of the front.
func serveTLSFront(t *testing.T, unixAddr, certPath, keyPath string) string {
	t.Helper()

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	sockPath := strings.TrimPrefix(unixAddr, "unix:")
	dialer := &net.Dialer{}
	ctx := t.Context()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed at test cleanup
			}
			go func() {
				defer func() { _ = conn.Close() }()
				backend, err := dialer.DialContext(ctx, "unix", sockPath)
				if err != nil {
					return
				}
				defer func() { _ = backend.Close() }()
				go func() { _, _ = io.Copy(backend, conn) }()
				_, _ = io.Copy(conn, backend)
			}()
		}
	}()

	return fmt.Sprintf("ssl:%s", ln.Addr().String())
}

// TestConnect_SSLEndpoints proves that ssl: NB/SB addresses are dialed with the
// configured TLS material — before this, only tcp: was wired and an ssl:
// deployment could not be monitored at all.
func TestConnect_SSLEndpoints(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	dir := t.TempDir()
	certPath, keyPath := writeTestCert(t, dir)
	nbSSL := serveTLSFront(t, nbAddr, certPath, keyPath)
	sbSSL := serveTLSFront(t, sbAddr, certPath, keyPath)

	tlsCfg, err := BuildTLSConfig(certPath, keyPath, certPath)
	require.NoError(t, err)
	require.NotNil(t, tlsCfg)

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// The TLS front does not expose "_Server", so skip the Raft monitors.
	dbs, err := Connect(ctx, nbSSL, sbSSL, nbModel, sbModel,
		MonitorOptions{SkipServerMonitors: true}, tlsCfg, tlsCfg)
	require.NoError(t, err)
	defer dbs.Close()

	assert.True(t, dbs.Ready())
}

// TestConnect_SSLNoTLS is the error path: without TLS material the handshake
// against an ssl: endpoint cannot complete, and Connect must fail rather than
// fall back to cleartext.
func TestConnect_SSLNoTLS(t *testing.T) {
	nbAddr, nbCleanup := setupNBServer(t)
	defer nbCleanup()
	sbAddr, sbCleanup := setupSBServer(t)
	defer sbCleanup()

	dir := t.TempDir()
	certPath, keyPath := writeTestCert(t, dir)
	nbSSL := serveTLSFront(t, nbAddr, certPath, keyPath)
	sbSSL := serveTLSFront(t, sbAddr, certPath, keyPath)

	nbModel, err := nb.FullDatabaseModel()
	require.NoError(t, err)
	sbModel, err := sb.FullDatabaseModel()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = Connect(ctx, nbSSL, sbSSL, nbModel, sbModel,
		MonitorOptions{SkipServerMonitors: true}, nil, nil)
	require.Error(t, err)
}
