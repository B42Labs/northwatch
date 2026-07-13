package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/b42labs/northwatch/internal/openapi"
)

// RegisterOpenAPI registers the OpenAPI spec and documentation endpoints.
// Must be called before RegisterAPICatchAll.
func RegisterOpenAPI(mux *http.ServeMux, spec openapi.Document) {
	specJSON, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		slog.Error("marshaling openapi spec failed", "err", err)
		return
	}

	mux.HandleFunc("GET /api/v1/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(specJSON)
	})

	mux.HandleFunc("GET /api/v1/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Security-Policy", docsCSP)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = fmt.Fprint(w, scalarHTML)
	})
}

// docsCSP bounds what the documentation page may load. The SPA has had a strict
// policy all along; this page had none while pulling a script off a public CDN.
//
// The directives track what the pinned Scalar bundle actually needs: its script
// from jsdelivr, its web fonts, the styles it injects at runtime, a blob: worker,
// and a same-origin fetch of the spec.
//
// connect-src is deliberately 'self' only. The bundle also tries to reach
// api.scalar.com for its "Ask AI" search registry; blocking that keeps the API
// surface of an internal deployment from being sent to a third party, and the
// reference renders without it.
const docsCSP = "default-src 'self'; " +
	"script-src 'self' https://cdn.jsdelivr.net; " +
	"worker-src 'self' blob:; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"font-src 'self' data: https://fonts.scalar.com; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'"

// scalarVersion pins the API-reference bundle. An unpinned "@latest" URL means a
// third party can change the script running on this origin at any time; the
// integrity hash then makes a substituted bundle fail to execute rather than run.
//
// To move the pin: bump the version and recompute the hash with
//
//	curl -sfL <url> | openssl dgst -sha384 -binary | openssl base64 -A
const (
	scalarVersion   = "1.62.5"
	scalarIntegrity = "sha384-jVBCKhcCfx34USN27x4iQK1SBNdL/HxKq3KuBAxTS4WPaP5w80K4fjpwB+DezJL5"
)

var scalarHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Northwatch API Documentation</title>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
  <script id="api-reference" data-url="/api/v1/openapi.json"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@` + scalarVersion + `/dist/browser/standalone.min.js"
          integrity="` + scalarIntegrity + `"
          crossorigin="anonymous"></script>
</body>
</html>`
