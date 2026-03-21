package handler

import (
	"net/http"

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
		summary, err := diagnoser.DiagnoseAll(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
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
