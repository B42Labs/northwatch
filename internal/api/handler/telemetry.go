package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/telemetry"
)

// RegisterTelemetry registers telemetry REST endpoints and the Prometheus scrape endpoint.
// If registry is nil, the /metrics endpoint is not registered.
func RegisterTelemetry(mux *http.ServeMux, querier *telemetry.Querier, registry *prometheus.Registry, propStore *telemetry.PropagationStore) {
	mux.HandleFunc("GET /api/v1/telemetry/summary", func(w http.ResponseWriter, r *http.Request) {
		result, err := querier.Summary(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("GET /api/v1/telemetry/flows", func(w http.ResponseWriter, r *http.Request) {
		result, err := querier.FlowMetrics(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("GET /api/v1/telemetry/propagation", func(w http.ResponseWriter, r *http.Request) {
		result, err := querier.Propagation(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("GET /api/v1/telemetry/cluster", func(w http.ResponseWriter, r *http.Request) {
		result, err := querier.Cluster(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("GET /api/v1/telemetry/raft-health", handleRaftHealth(querier))

	if propStore != nil {
		mux.HandleFunc("GET /api/v1/telemetry/propagation/timeline", handlePropagationTimeline(querier, propStore))
		mux.HandleFunc("GET /api/v1/telemetry/propagation/heatmap", handlePropagationHeatmap(querier, propStore))
	}

	if registry != nil {
		mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	}
}

func handlePropagationTimeline(querier *telemetry.Querier, store *telemetry.PropagationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chassis := r.URL.Query().Get("chassis")
		since, err := parseIntParam(r, "since", 0)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid since parameter")
			return
		}
		limit, err := parseIntParam(r, "limit", 1000)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}

		events := store.Query(chassis, since)
		if limit > 0 && len(events) > int(limit) {
			events = events[len(events)-int(limit):]
		}

		var currentGen int
		if prop, err := querier.Propagation(r.Context()); err == nil {
			currentGen = prop.NbCfg
		}

		api.WriteJSON(w, http.StatusOK, map[string]any{
			"events":             events,
			"current_generation": currentGen,
			"count":              len(events),
		})
	}
}

func handlePropagationHeatmap(querier *telemetry.Querier, store *telemetry.PropagationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		since, err := parseIntParam(r, "since", 0)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid since parameter")
			return
		}
		if since == 0 {
			since = time.Now().Add(-time.Hour).UnixMilli()
		}

		summaries := store.Summary(since)

		var currentGen int
		if prop, err := querier.Propagation(r.Context()); err == nil {
			currentGen = prop.NbCfg
		}

		api.WriteJSON(w, http.StatusOK, map[string]any{
			"chassis":            summaries,
			"current_generation": currentGen,
			"since":              since,
		})
	}
}

func parseIntParam(r *http.Request, key string, defaultVal int64) (int64, error) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func handleRaftHealth(querier *telemetry.Querier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := querier.RaftHealth(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, result)
	}
}
