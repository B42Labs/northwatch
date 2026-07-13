package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer_WiringAndAccessors(t *testing.T) {
	// A wrapper records that it saw the request, proving NewServer composes the
	// wrappers around the mux. dbs is nil, so the stale predicate stays disabled.
	var wrapped bool
	wrapper := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrapped = true
			next.ServeHTTP(w, r)
		})
	}

	s := NewServer("127.0.0.1:0", nil, wrapper)
	require.NotNil(t, s.Mux())
	assert.Nil(t, s.Databases(), "Databases returns the (nil) bundle it was built with")

	s.Mux().HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	w := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(w, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", nil))

	assert.True(t, wrapped, "the wrapper must run around the mux")
	assert.Equal(t, http.StatusTeapot, w.Code)
}

func TestServer_ListenAndServeThenShutdown(t *testing.T) {
	s := NewServer("127.0.0.1:0", nil)

	errCh := make(chan error, 1)
	go func() { errCh <- s.ListenAndServe(context.Background()) }()

	// Shutdown is idempotent; retry it until ListenAndServe returns. This avoids
	// racing the bind and needs no sleep.
	var serveErr error
	require.Eventually(t, func() bool {
		_ = s.Shutdown(context.Background())
		select {
		case serveErr = <-errCh:
			return true
		default:
			return false
		}
	}, 5*time.Second, 20*time.Millisecond)

	assert.ErrorIs(t, serveErr, http.ErrServerClosed)
}

func TestServer_ListenAndServeBindError(t *testing.T) {
	// A port outside the valid range cannot be bound, so ListenAndServe returns
	// the wrapped listen error rather than blocking in Serve.
	s := NewServer("127.0.0.1:99999999", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := s.ListenAndServe(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listen:")
}

func TestWriteJSONList_NilCoercedToEmptyArray(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSONList[string](w, http.StatusOK, nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, "[]", w.Body.String())
}

func TestWriteJSONList_ItemsRendered(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSONList(w, http.StatusOK, []string{"a", "b"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `["a","b"]`, w.Body.String())
}

func TestNonNil(t *testing.T) {
	assert.Equal(t, []int{}, NonNil[int](nil), "nil slice becomes an empty, non-nil slice")

	in := []int{1, 2}
	assert.Equal(t, in, NonNil(in), "a non-nil slice is returned unchanged")
}

// writeServerCert generates a self-signed certificate for 127.0.0.1, writes the
// PEM pair into a temp dir, and returns the paths plus a cert pool trusting it.
func writeServerCert(t *testing.T) (certPath, keyPath string, pool *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "northwatch-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	require.NoError(t, os.WriteFile(certPath,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPath = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(keyPath,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))

	pool = x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))

	return certPath, keyPath, pool
}

func TestServer_ServeTLS(t *testing.T) {
	certPath, keyPath, pool := writeServerCert(t)

	s := NewServer("127.0.0.1:0", nil)
	s.SetTLSFiles(certPath, keyPath)
	s.Mux().HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Bind a listener first so the test knows the port, then hand its address to
	// the server (ListenAndServe binds Addr itself).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	s.httpServer.Addr = addr

	errCh := make(chan error, 1)
	go func() { errCh <- s.ListenAndServe(context.Background()) }()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(shutdownCtx)
		<-errCh
	})

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
	}}

	var resp *http.Response
	require.Eventually(t, func() bool {
		r, err := client.Get("https://" + addr + "/ping")
		if err != nil {
			return false
		}
		resp = r
		return true
	}, 5*time.Second, 20*time.Millisecond)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, resp.TLS)
	assert.GreaterOrEqual(t, resp.TLS.Version, uint16(tls.VersionTLS12))
}

func TestServer_ServeTLS_MissingCertFile(t *testing.T) {
	// A cert path that does not exist must fail the serve loop loudly rather than
	// silently falling back to plain HTTP.
	s := NewServer("127.0.0.1:0", nil)
	s.SetTLSFiles(filepath.Join(t.TempDir(), "absent.pem"), filepath.Join(t.TempDir(), "absent-key.pem"))

	err := s.ListenAndServe(context.Background())
	require.Error(t, err)
	assert.NotErrorIs(t, err, http.ErrServerClosed)
}
