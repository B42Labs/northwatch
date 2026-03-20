package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/history"
)

// RegisterHistory registers all history/snapshot endpoints.
func RegisterHistory(mux *http.ServeMux, store *history.Store, collector *history.Collector) {
	mux.HandleFunc("GET /api/v1/snapshots", handleListSnapshots(store))
	mux.HandleFunc("POST /api/v1/snapshots", handleCreateSnapshot(collector))
	mux.HandleFunc("GET /api/v1/snapshots/diff", handleDiffSnapshots(store))
	mux.HandleFunc("GET /api/v1/snapshots/{id}", handleGetSnapshot(store))
	mux.HandleFunc("GET /api/v1/snapshots/{id}/rows", handleGetSnapshotRows(store))
	mux.HandleFunc("DELETE /api/v1/snapshots/{id}", handleDeleteSnapshot(store))
	mux.HandleFunc("GET /api/v1/events", handleQueryEvents(store))
}

func handleListSnapshots(store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := store.ListSnapshots(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if list == nil {
			list = []history.SnapshotMeta{}
		}
		api.WriteJSON(w, http.StatusOK, list)
	}
}

func handleCreateSnapshot(collector *history.Collector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Label string `json:"label"`
		}
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
		}

		meta, err := collector.TakeSnapshot(r.Context(), "manual", body.Label)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusCreated, meta)
	}
}

func handleDiffSnapshots(store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fromStr := r.URL.Query().Get("from")
		toStr := r.URL.Query().Get("to")
		if fromStr == "" || toStr == "" {
			api.WriteError(w, http.StatusBadRequest, "from and to query parameters are required")
			return
		}

		fromID, err := strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid from parameter")
			return
		}
		toID, err := strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid to parameter")
			return
		}

		tableFilter := r.URL.Query().Get("table")

		diff, err := store.DiffSnapshots(r.Context(), fromID, toID, tableFilter)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, diff)
	}
}

func handleGetSnapshot(store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid snapshot id")
			return
		}

		meta, err := store.GetSnapshot(r.Context(), id)
		if err != nil {
			if errors.Is(err, history.ErrNotFound) {
				api.WriteError(w, http.StatusNotFound, "snapshot not found")
			} else {
				api.WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		api.WriteJSON(w, http.StatusOK, meta)
	}
}

func handleGetSnapshotRows(store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid snapshot id")
			return
		}

		database := r.URL.Query().Get("database")
		table := r.URL.Query().Get("table")

		rows, err := store.GetSnapshotRows(r.Context(), id, database, table)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if rows == nil {
			rows = []history.SnapshotRow{}
		}
		api.WriteJSON(w, http.StatusOK, rows)
	}
}

func handleDeleteSnapshot(store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid snapshot id")
			return
		}

		if err := store.DeleteSnapshot(r.Context(), id); err != nil {
			if errors.Is(err, history.ErrNotFound) {
				api.WriteError(w, http.StatusNotFound, "snapshot not found")
			} else {
				api.WriteError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleQueryEvents(store *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		opts := history.EventQueryOpts{
			Database: q.Get("database"),
			Table:    q.Get("table"),
			Type:     q.Get("type"),
		}

		if v := q.Get("limit"); v != "" {
			limit, err := strconv.Atoi(v)
			if err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid limit parameter")
				return
			}
			opts.Limit = limit
		}

		if v := q.Get("since"); v != "" {
			t, err := parseTime(v)
			if err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid since parameter: use RFC3339 or Unix millis")
				return
			}
			opts.Since = &t
		}
		if v := q.Get("until"); v != "" {
			t, err := parseTime(v)
			if err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid until parameter: use RFC3339 or Unix millis")
				return
			}
			opts.Until = &t
		}

		events, err := store.QueryEvents(r.Context(), opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if events == nil {
			events = []history.EventRecord{}
		}
		api.WriteJSON(w, http.StatusOK, events)
	}
}

// parseTime accepts RFC3339 or Unix milliseconds.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	ms, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms), nil
}
