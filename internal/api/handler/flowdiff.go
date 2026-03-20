package handler

import (
	"net/http"
	"strconv"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/flowdiff"
)

// FlowDiffResponse is the JSON response for GET /api/v1/debug/flow-diff.
type FlowDiffResponse struct {
	Changes []flowdiff.FlowChange `json:"changes"`
	Count   int                   `json:"count"`
}

// RegisterFlowDiff registers the flow diff endpoint.
func RegisterFlowDiff(mux *http.ServeMux, store *flowdiff.Store) {
	mux.HandleFunc("GET /api/v1/debug/flow-diff", handleFlowDiff(store))
}

func handleFlowDiff(store *flowdiff.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		datapath := r.URL.Query().Get("datapath")
		var since int64
		if s := r.URL.Query().Get("since"); s != "" {
			parsed, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid since parameter")
				return
			}
			since = parsed
		}

		changes := store.Query(datapath, since)
		if changes == nil {
			changes = []flowdiff.FlowChange{}
		}

		api.WriteJSON(w, http.StatusOK, FlowDiffResponse{
			Changes: changes,
			Count:   len(changes),
		})
	}
}
