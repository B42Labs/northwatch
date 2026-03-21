package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
)

// RegisterCapabilities registers the capabilities endpoint.
// enrichEnabled indicates whether an enrichment provider is configured.
// writeEnabled indicates whether write operations are enabled.
// multiCluster indicates whether multiple clusters are configured.
func RegisterCapabilities(mux *http.ServeMux, enrichEnabled, writeEnabled, multiCluster bool) {
	caps := []string{"read", "debug", "correlate", "realtime", "topology", "flows", "telemetry", "alerts", "history", "openapi"}
	if enrichEnabled {
		caps = append(caps, "enrich")
	}
	if writeEnabled {
		caps = append(caps, "write")
	}
	if multiCluster {
		caps = append(caps, "multi-cluster")
	}

	mux.HandleFunc("GET /api/v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, map[string]any{
			"capabilities": caps,
		})
	})
}
