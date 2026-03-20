package handler

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/b42labs/northwatch/internal/api"
)

// RegisterAPICatchAll registers a 404 handler for unmatched /api/ paths.
// This prevents API requests from falling through to the SPA handler and
// returning HTML, which causes confusing JSON parse errors in the browser.
// Must be registered before RegisterUI.
func RegisterAPICatchAll(mux *http.ServeMux) {
	apiNotFound := func(w http.ResponseWriter, r *http.Request) {
		api.WriteError(w, http.StatusNotFound, "not found")
	}
	mux.HandleFunc("GET /api/", apiNotFound)
	mux.HandleFunc("POST /api/", apiNotFound)
	mux.HandleFunc("PUT /api/", apiNotFound)
	mux.HandleFunc("PATCH /api/", apiNotFound)
	mux.HandleFunc("DELETE /api/", apiNotFound)
}

// RegisterUI serves the embedded SPA frontend.
// It must be registered after all API routes since GET / is the lowest-priority pattern.
func RegisterUI(mux *http.ServeMux, distFS fs.FS) {
	sub, err := fs.Sub(distFS, "frontend/dist")
	if err != nil {
		// Fall back to the root FS if the subdirectory doesn't exist.
		sub = distFS
	}

	// Read index.html once at startup instead of per-request.
	indexData, indexErr := fs.ReadFile(sub, "index.html")

	fileServer := http.FileServerFS(sub)

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Path traversal is safe here: net/http.ServeMux canonicalizes the URL
		// path before invoking the handler, stripping ".." segments.
		path := r.URL.Path
		if path != "/" {
			if f, err := sub.Open(path[1:]); err == nil {
				_ = f.Close()
				// Vite hashed assets get long cache; other files don't.
				if strings.HasPrefix(path, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback: serve index.html for all unknown paths.
		if indexErr != nil {
			http.Error(w, "UI not built. Run: make build-ui", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'")
		_, _ = w.Write(indexData)
	})
}
