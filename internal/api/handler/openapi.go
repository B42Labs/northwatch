package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/b42labs/northwatch/internal/openapi"
)

// RegisterOpenAPI registers the OpenAPI spec and documentation endpoints.
// Must be called before RegisterAPICatchAll.
func RegisterOpenAPI(mux *http.ServeMux, spec openapi.Document) {
	specJSON, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		log.Printf("openapi: failed to marshal spec: %v", err)
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
		_, _ = fmt.Fprint(w, scalarHTML)
	})
}

const scalarHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Northwatch API Documentation</title>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
  <script id="api-reference" data-url="/api/v1/openapi.json"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`
