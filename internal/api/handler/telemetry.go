package handler

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/telemetry"
)

// RegisterTelemetry registers telemetry REST endpoints and the Prometheus scrape endpoint.
func RegisterTelemetry(mux *http.ServeMux, querier *telemetry.Querier, registry *prometheus.Registry) {
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

	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
}
