package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/history"
	"github.com/b42labs/northwatch/internal/snapshotsession"
)

// RegisterSnapshotLoad registers the endpoints that load a stored history
// snapshot as a runtime data source and unload it again. They live on the main
// mux (not a cluster sub-mux): managing snapshot clusters is a global action.
func RegisterSnapshotLoad(mux *http.ServeMux, mgr *snapshotsession.Manager) {
	mux.HandleFunc("POST /api/v1/snapshots/{id}/load", handleLoadSnapshot(mgr))
	mux.HandleFunc("POST /api/v1/snapshots/{id}/unload", handleUnloadSnapshot(mgr))
}

func handleLoadSnapshot(mgr *snapshotsession.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid snapshot id")
			return
		}
		ld, err := mgr.Load(r.Context(), id)
		if err != nil {
			switch {
			case errors.Is(err, history.ErrNotFound):
				api.WriteError(w, http.StatusNotFound, "snapshot not found")
			case errors.Is(err, snapshotsession.ErrTooManyLoaded),
				errors.Is(err, snapshotsession.ErrLoadInProgress):
				api.WriteError(w, http.StatusConflict, err.Error())
			default:
				api.WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]any{
			"cluster": ld.Cluster,
			"label":   ld.Label,
			"mode":    ld.Mode,
			"snapshot": map[string]any{
				"sourceId":  ld.SourceID,
				"createdAt": ld.CreatedAt,
			},
		})
	}
}

func handleUnloadSnapshot(mgr *snapshotsession.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid snapshot id")
			return
		}
		if err := mgr.Unload(r.Context(), id); err != nil {
			if errors.Is(err, snapshotsession.ErrNotLoaded) {
				api.WriteError(w, http.StatusNotFound, "snapshot not loaded")
			} else {
				api.WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
