package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/inventory"
)

// RegisterInventory registers the aggregated, chassis-centric SB inventory
// endpoints. The view is computed entirely from the existing Southbound cache;
// staleThreshold bounds how fresh a chassis nb_cfg_timestamp must be for the
// chassis to be reported alive.
func RegisterInventory(mux *http.ServeMux, sbClient client.Client, staleThreshold time.Duration) {
	b := &inventory.Builder{SB: sbClient, StaleThreshold: staleThreshold, Now: time.Now}

	mux.HandleFunc("GET /api/v1/sb/chassis-inventory", func(w http.ResponseWriter, r *http.Request) {
		summaries, err := b.List(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		api.WriteJSON(w, http.StatusOK, summaries)
	})

	mux.HandleFunc("GET /api/v1/sb/chassis-inventory/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			api.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		detail, err := b.Detail(r.Context(), name)
		if err != nil {
			if errors.Is(err, inventory.ErrNotFound) {
				api.WriteError(w, http.StatusNotFound, "not found")
				return
			}
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		api.WriteJSON(w, http.StatusOK, detail)
	})
}
