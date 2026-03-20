package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
)

// RegisterCapabilities registers the capabilities endpoint.
// enrichEnabled indicates whether an enrichment provider is configured.
func RegisterCapabilities(mux *http.ServeMux, enrichEnabled bool) {
	caps := []string{"read", "debug", "correlate"}
	if enrichEnabled {
		caps = append(caps, "enrich")
	}

	mux.HandleFunc("GET /api/v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, map[string]any{
			"capabilities": caps,
		})
	})
}
