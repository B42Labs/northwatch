package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
)

func RegisterCapabilities(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/capabilities", handleCapabilities)
}

func handleCapabilities(w http.ResponseWriter, r *http.Request) {
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"capabilities": []string{"read", "debug"},
	})
}
