package handler

import (
	"errors"
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/correlate"
	"github.com/b42labs/northwatch/internal/enrich"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

type correlatedHandler struct {
	correlator *correlate.Correlator
	enricher   *enrich.Enricher
}

// RegisterCorrelated registers all correlated API endpoints.
func RegisterCorrelated(mux *http.ServeMux, cor *correlate.Correlator, enricher *enrich.Enricher) {
	h := &correlatedHandler{correlator: cor, enricher: enricher}

	mux.HandleFunc("GET /api/v1/correlated/logical-switches", h.listSwitches)
	mux.HandleFunc("GET /api/v1/correlated/logical-switches/{uuid}", h.getSwitch)
	mux.HandleFunc("GET /api/v1/correlated/logical-routers", h.listRouters)
	mux.HandleFunc("GET /api/v1/correlated/logical-routers/{uuid}", h.getRouter)
	mux.HandleFunc("GET /api/v1/correlated/logical-switch-ports/{uuid}", h.getLSP)
	mux.HandleFunc("GET /api/v1/correlated/logical-router-ports/{uuid}", h.getLRP)
	mux.HandleFunc("GET /api/v1/correlated/chassis", h.listChassis)
	mux.HandleFunc("GET /api/v1/correlated/chassis/{uuid}", h.getChassis)
	mux.HandleFunc("GET /api/v1/correlated/port-bindings/{uuid}", h.getPortBinding)
}

func (h *correlatedHandler) listSwitches(w http.ResponseWriter, r *http.Request) {
	var switches []nb.LogicalSwitch
	if err := h.correlator.NB.List(r.Context(), &switches); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	results := make([]correlate.SwitchCorrelated, 0, len(switches))
	for _, ls := range switches {
		results = append(results, h.correlator.SwitchSummary(r.Context(), ls))
	}

	api.WriteJSON(w, http.StatusOK, results)
}

func (h *correlatedHandler) getSwitch(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "uuid is required")
		return
	}

	result, err := h.correlator.SwitchDetail(r.Context(), uuid)
	if err != nil {
		writeCorrelateError(w, err)
		return
	}

	h.enrichSwitch(r, result)
	api.WriteJSON(w, http.StatusOK, result)
}

func (h *correlatedHandler) listRouters(w http.ResponseWriter, r *http.Request) {
	var routers []nb.LogicalRouter
	if err := h.correlator.NB.List(r.Context(), &routers); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	results := make([]correlate.RouterCorrelated, 0, len(routers))
	for _, lr := range routers {
		results = append(results, h.correlator.RouterSummary(r.Context(), lr))
	}

	api.WriteJSON(w, http.StatusOK, results)
}

func (h *correlatedHandler) getRouter(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "uuid is required")
		return
	}

	result, err := h.correlator.RouterDetail(r.Context(), uuid)
	if err != nil {
		writeCorrelateError(w, err)
		return
	}

	h.enrichRouter(r, result)
	api.WriteJSON(w, http.StatusOK, result)
}

func (h *correlatedHandler) getLSP(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "uuid is required")
		return
	}

	chain, err := h.correlator.LSPDetail(r.Context(), uuid)
	if err != nil {
		writeCorrelateError(w, err)
		return
	}

	h.enrichPortChain(r, chain)
	api.WriteJSON(w, http.StatusOK, chain)
}

func (h *correlatedHandler) getLRP(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "uuid is required")
		return
	}

	chain, err := h.correlator.LRPDetail(r.Context(), uuid)
	if err != nil {
		writeCorrelateError(w, err)
		return
	}

	// LRP enrichment is skipped: router ports typically lack neutron:* external_ids.
	api.WriteJSON(w, http.StatusOK, chain)
}

func (h *correlatedHandler) listChassis(w http.ResponseWriter, r *http.Request) {
	var chassisList []sb.Chassis
	if err := h.correlator.SB.List(r.Context(), &chassisList); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	results := make([]correlate.ChassisCorrelated, 0, len(chassisList))
	for _, ch := range chassisList {
		results = append(results, h.correlator.ChassisSummary(r.Context(), ch))
	}

	api.WriteJSON(w, http.StatusOK, results)
}

func (h *correlatedHandler) getChassis(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "uuid is required")
		return
	}

	result, err := h.correlator.ChassisDetail(r.Context(), uuid)
	if err != nil {
		writeCorrelateError(w, err)
		return
	}

	api.WriteJSON(w, http.StatusOK, result)
}

func (h *correlatedHandler) getPortBinding(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	if uuid == "" {
		api.WriteError(w, http.StatusBadRequest, "uuid is required")
		return
	}

	chain, err := h.correlator.PortBindingDetail(r.Context(), uuid)
	if err != nil {
		writeCorrelateError(w, err)
		return
	}

	h.enrichPortChain(r, chain)
	api.WriteJSON(w, http.StatusOK, chain)
}

// enrichSwitch adds enrichment data to a SwitchCorrelated response.
func (h *correlatedHandler) enrichSwitch(r *http.Request, result *correlate.SwitchCorrelated) {
	if !h.enricher.HasProvider() {
		return
	}

	if eids := getExternalIDs(result.Switch); eids != nil {
		uuid, _ := result.Switch["_uuid"].(string)
		if info := h.enricher.EnrichNetwork(r.Context(), uuid, eids); info != nil {
			result.Switch["enrichment"] = info
		}
	}

	for i := range result.Ports {
		h.enrichPortChain(r, &result.Ports[i])
	}
}

// enrichRouter adds enrichment data to a RouterCorrelated response.
func (h *correlatedHandler) enrichRouter(r *http.Request, result *correlate.RouterCorrelated) {
	if !h.enricher.HasProvider() {
		return
	}

	if eids := getExternalIDs(result.Router); eids != nil {
		uuid, _ := result.Router["_uuid"].(string)
		if info := h.enricher.EnrichRouter(r.Context(), uuid, eids); info != nil {
			result.Router["enrichment"] = info
		}
	}

	for i := range result.Ports {
		h.enrichPortChain(r, &result.Ports[i])
	}

	for i, nat := range result.NATs {
		if eids := getExternalIDs(nat); eids != nil {
			uuid, _ := nat["_uuid"].(string)
			if info := h.enricher.EnrichNAT(r.Context(), uuid, eids); info != nil {
				result.NATs[i]["enrichment"] = info
			}
		}
	}
}

// enrichPortChain adds enrichment data to a PortBindingChain.
// Only LSP is enriched because router ports (LRP) typically lack neutron:* external_ids.
func (h *correlatedHandler) enrichPortChain(r *http.Request, chain *correlate.PortBindingChain) {
	if !h.enricher.HasProvider() {
		return
	}

	if chain.LSP != nil {
		if eids := getExternalIDs(chain.LSP); eids != nil {
			uuid, _ := chain.LSP["_uuid"].(string)
			if info := h.enricher.EnrichPort(r.Context(), uuid, eids); info != nil {
				chain.LSP["enrichment"] = info
			}
		}
	}
}

// writeCorrelateError writes 404 for not-found errors, 500 for everything else.
func writeCorrelateError(w http.ResponseWriter, err error) {
	if errors.Is(err, correlate.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	api.WriteError(w, http.StatusInternalServerError, "internal error")
}

// getExternalIDs extracts the external_ids map from a model map.
func getExternalIDs(m map[string]any) map[string]string {
	raw, ok := m["external_ids"]
	if !ok {
		return nil
	}
	eids, ok := raw.(map[string]string)
	if !ok {
		return nil
	}
	return eids
}
