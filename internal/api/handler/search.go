package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/search"
)

func RegisterSearch(mux *http.ServeMux, engine *search.Engine) {
	mux.HandleFunc("GET /api/v1/search", handleSearch(engine))
}

func handleSearch(engine *search.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			api.WriteError(w, http.StatusBadRequest, "query parameter 'q' is required")
			return
		}

		results, err := engine.Search(r.Context(), q)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		api.WriteJSON(w, http.StatusOK, map[string]any{
			"query":      q,
			"query_type": string(search.ClassifyQuery(q)),
			"results":    results,
		})
	}
}
