package ovsdb

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTLSConfig_Empty(t *testing.T) {
	// All three empty means no TLS material: plain tcp: (or default ssl:).
	cfg, err := BuildTLSConfig("", "", "")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestBuildTLSConfig_PartialIsError(t *testing.T) {
	tests := []struct {
		name             string
		cert, key, caArg string
	}{
		{"only cert", "c.pem", "", ""},
		{"only key", "", "k.pem", ""},
		{"only ca", "", "", "ca.pem"},
		{"cert+key no ca", "c.pem", "k.pem", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := BuildTLSConfig(tt.cert, tt.key, tt.caArg)
			require.Error(t, err)
			assert.Nil(t, cfg)
		})
	}
}

func TestBuildTLSConfig_BadPath(t *testing.T) {
	dir := t.TempDir()
	cfg, err := BuildTLSConfig(
		filepath.Join(dir, "missing-cert.pem"),
		filepath.Join(dir, "missing-key.pem"),
		filepath.Join(dir, "missing-ca.pem"),
	)
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestBuildTLSConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeSelfSignedTLS(t, dir)

	cfg, err := BuildTLSConfig(certPath, keyPath, caPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.NotNil(t, cfg.RootCAs)
	assert.Len(t, cfg.Certificates, 1)
}

func TestBuildTLSConfig_EmptyCABundle(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, _ := writeSelfSignedTLS(t, dir)
	caPath := filepath.Join(dir, "empty-ca.pem")
	require.NoError(t, os.WriteFile(caPath, []byte("not a certificate"), 0600))

	cfg, err := BuildTLSConfig(certPath, keyPath, caPath)
	require.Error(t, err)
	assert.Nil(t, cfg)
}

// writeSelfSignedTLS generates a self-signed ECDSA certificate and writes the
// cert, key and (cert-as-)CA bundle to dir, returning their paths.
func writeSelfSignedTLS(t *testing.T, dir string) (certPath, keyPath, caPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "northwatch-test"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(1<<31-1, 0),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "cert.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	require.NoError(t, os.WriteFile(certPath, certPEM, 0600))

	keyPath = filepath.Join(dir, "key.pem")
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0600))

	// The CA bundle is the same self-signed cert.
	caPath = filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(caPath, certPEM, 0600))
	return certPath, keyPath, caPath
}
