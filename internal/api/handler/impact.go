package handler

import (
	"errors"
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/impact"
)

// RegisterImpact registers the impact analysis HTTP endpoint.
func RegisterImpact(mux *http.ServeMux, resolver *impact.Resolver) {
	mux.HandleFunc("GET /api/v1/impact/{db}/{table}/{uuid}", handleImpact(resolver))
}

func handleImpact(resolver *impact.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := r.PathValue("db")
		table := r.PathValue("table")
		uuid := r.PathValue("uuid")

		if db == "" || table == "" || uuid == "" {
			api.WriteError(w, http.StatusBadRequest, "db, table, and uuid are required")
			return
		}

		result, err := resolver.Resolve(r.Context(), db, table, uuid)
		if err != nil {
			switch {
			case errors.Is(err, impact.ErrUnsupportedDB), errors.Is(err, impact.ErrUnknownTable):
				api.WriteError(w, http.StatusBadRequest, err.Error())
			default:
				api.WriteError(w, http.StatusNotFound, err.Error())
			}
			return
		}

		api.WriteJSON(w, http.StatusOK, result)
	}
}
