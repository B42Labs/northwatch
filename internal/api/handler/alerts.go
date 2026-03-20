package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/alert"
	"github.com/b42labs/northwatch/internal/api"
)

// RegisterAlerts registers alert REST endpoints.
func RegisterAlerts(mux *http.ServeMux, engine *alert.Engine) {
	mux.HandleFunc("GET /api/v1/alerts", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, engine.ActiveAlerts())
	})

	mux.HandleFunc("GET /api/v1/alerts/rules", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, engine.Rules())
	})
}
