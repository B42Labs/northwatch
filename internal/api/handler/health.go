package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
)

// ReadinessChecker reports whether the system is ready to serve traffic.
type ReadinessChecker interface {
	Ready() bool
}

func RegisterHealth(mux *http.ServeMux, rc ReadinessChecker) {
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", handleReadyzFunc(rc))
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleReadyzFunc(rc ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rc.Ready() {
			api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		} else {
			api.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
		}
	}
}
