package handler

import (
	"net/http"
	"strconv"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/debug"
)

// RegisterDebug registers the debug tool endpoints.
func RegisterDebug(mux *http.ServeMux, checker *debug.ConnectivityChecker, diagnoser *debug.PortDiagnoser, auditor *debug.ACLAuditor, detector *debug.StaleDetector) {
	mux.HandleFunc("GET /api/v1/debug/port-diagnostics", handlePortDiagnostics(diagnoser))
	mux.HandleFunc("GET /api/v1/debug/port-diagnostics/{uuid}", handleSinglePortDiagnostic(diagnoser))
	mux.HandleFunc("GET /api/v1/debug/connectivity", handleConnectivity(checker))
	mux.HandleFunc("GET /api/v1/debug/acl-audit", handleACLAudit(auditor))
	mux.HandleFunc("GET /api/v1/debug/stale-entries", handleStaleEntries(detector))
}

func handlePortDiagnostics(diagnoser *debug.PortDiagnoser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		severity := r.URL.Query().Get("severity")
		switch severity {
		case "", "healthy", "warning", "error":
		default:
			api.WriteError(w, http.StatusBadRequest, "severity must be one of: healthy, warning, error")
			return
		}

		limit := -1
		if s := r.URL.Query().Get("limit"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				api.WriteError(w, http.StatusBadRequest, "limit must be a non-negative integer")
				return
			}
			limit = n
		}

		summary, err := diagnoser.DiagnoseAll(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// The Total/Healthy/Warning/Error counts always reflect the full
		// diagnosis; severity and limit only filter and cap the returned Ports.
		if severity != "" {
			filtered := make([]debug.PortDiagnostic, 0, len(summary.Ports))
			for _, p := range summary.Ports {
				if string(p.Overall) == severity {
					filtered = append(filtered, p)
				}
			}
			summary.Ports = filtered
		}
		if limit >= 0 && len(summary.Ports) > limit {
			summary.Ports = summary.Ports[:limit]
		}

		api.WriteJSON(w, http.StatusOK, summary)
	}
}

func handleSinglePortDiagnostic(diagnoser *debug.PortDiagnoser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuid := r.PathValue("uuid")
		diag, err := diagnoser.DiagnosePort(r.Context(), uuid)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, diag)
	}
}

func handleConnectivity(checker *debug.ConnectivityChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		srcUUID := r.URL.Query().Get("src")
		dstUUID := r.URL.Query().Get("dst")

		if srcUUID == "" || dstUUID == "" {
			api.WriteError(w, http.StatusBadRequest, "src and dst query parameters are required")
			return
		}

		result, err := checker.Check(r.Context(), srcUUID, dstUUID)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, result)
	}
}

func handleACLAudit(auditor *debug.ACLAuditor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := auditor.Audit(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, result)
	}
}

func handleStaleEntries(detector *debug.StaleDetector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := detector.DetectAll(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, result)
	}
}
