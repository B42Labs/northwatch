package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestRegisterUI_ServesIndexAtRoot(t *testing.T) {
	fs := fstest.MapFS{
		"frontend/dist/index.html": &fstest.MapFile{
			Data: []byte("<html>northwatch</html>"),
		},
	}

	mux := http.NewServeMux()
	RegisterUI(mux, fs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "northwatch")
	assert.Equal(t, "no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
	assert.Equal(t, "default-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'", w.Header().Get("Content-Security-Policy"))
}

func TestRegisterUI_ServesStaticAssets(t *testing.T) {
	fs := fstest.MapFS{
		"frontend/dist/index.html": &fstest.MapFile{
			Data: []byte("<html></html>"),
		},
		"frontend/dist/assets/app-abc123.js": &fstest.MapFile{
			Data: []byte("console.log('app')"),
		},
	}

	mux := http.NewServeMux()
	RegisterUI(mux, fs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/assets/app-abc123.js", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "console.log")
	assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
}

func TestRegisterUI_SPAFallback(t *testing.T) {
	fs := fstest.MapFS{
		"frontend/dist/index.html": &fstest.MapFile{
			Data: []byte("<html>spa</html>"),
		},
	}

	mux := http.NewServeMux()
	RegisterUI(mux, fs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/some/deep/route", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "spa")
	assert.Equal(t, "no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
	assert.Equal(t, "default-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'", w.Header().Get("Content-Security-Policy"))
}

func TestRegisterUI_NotBuilt(t *testing.T) {
	// Empty FS — no index.html
	fs := fstest.MapFS{
		"frontend/dist/.gitkeep": &fstest.MapFile{Data: []byte{}},
	}

	mux := http.NewServeMux()
	RegisterUI(mux, fs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "make build-ui")
}

func TestRegisterUI_NonHashedAssetsNoCacheControl(t *testing.T) {
	fs := fstest.MapFS{
		"frontend/dist/index.html": &fstest.MapFile{
			Data: []byte("<html></html>"),
		},
		"frontend/dist/favicon.ico": &fstest.MapFile{
			Data: []byte("icon"),
		},
	}

	mux := http.NewServeMux()
	RegisterUI(mux, fs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/favicon.ico", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Cache-Control"))
}

func TestRegisterAPICatchAll_Returns404JSON(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAPICatchAll(mux)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/sb/logical-switches/some-uuid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"not found"`)
}

func TestRegisterUI_DistSubdirectory(t *testing.T) {
	// Verify assets inside frontend/dist are accessible under root paths.
	fs := fstest.MapFS{
		"frontend/dist/index.html": &fstest.MapFile{
			Data: []byte("<html>dist</html>"),
		},
		"frontend/dist/style.css": &fstest.MapFile{
			Data: []byte("body{}"),
		},
	}

	mux := http.NewServeMux()
	RegisterUI(mux, fs)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/style.css", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "body{}")
}
