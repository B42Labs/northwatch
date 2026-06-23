package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
)

// SnapshotInfo describes the captured snapshot backing the server when running
// in offline mode. It is surfaced to the UI so it can show a "snapshot" mode
// indicator instead of "live".
type SnapshotInfo struct {
	CreatedAt string `json:"createdAt,omitempty"`
	NBAddr    string `json:"nbAddr,omitempty"`
	SBAddr    string `json:"sbAddr,omitempty"`
}

// RegisterCapabilities registers the capabilities endpoint.
// enrichEnabled indicates whether an enrichment provider is configured.
// writeEnabled indicates whether write operations are enabled.
// multiCluster indicates whether multiple clusters are configured.
// snapshot is non-nil when the server is serving an offline snapshot.
func RegisterCapabilities(mux *http.ServeMux, enrichEnabled, writeEnabled, multiCluster bool, snapshot *SnapshotInfo) {
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

	mode := "live"
	if snapshot != nil {
		caps = append(caps, "snapshot")
		mode = "snapshot"
	}

	mux.HandleFunc("GET /api/v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"capabilities": caps,
			"mode":         mode,
		}
		if snapshot != nil {
			resp["snapshot"] = snapshot
		}
		api.WriteJSON(w, http.StatusOK, resp)
	})
}
