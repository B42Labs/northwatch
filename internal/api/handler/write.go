package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/write"
)

// maxWriteBodySize limits the size of write request bodies to 1 MB.
const maxWriteBodySize = 1 << 20

// RegisterWrite registers all write operation HTTP endpoints.
func RegisterWrite(mux *http.ServeMux, engine *write.Engine) {
	mux.HandleFunc("GET /api/v1/write/schema", handleSchema(engine))
	mux.HandleFunc("POST /api/v1/write/preview", handlePreview(engine))
	mux.HandleFunc("POST /api/v1/write/dry-run", handleDryRun(engine))
	mux.HandleFunc("GET /api/v1/write/plans/{id}", handleGetPlan(engine))
	mux.HandleFunc("POST /api/v1/write/plans/{id}/apply", handleApply(engine))
	mux.HandleFunc("DELETE /api/v1/write/plans/{id}", handleCancelPlan(engine))
	mux.HandleFunc("POST /api/v1/write/rollback", handleRollback(engine))
	mux.HandleFunc("GET /api/v1/write/audit", handleAuditLog(engine))
	mux.HandleFunc("GET /api/v1/write/audit/{id}", handleGetAuditEntry(engine))
}

// writeRequest is the common request body for preview and dry-run endpoints.
type writeRequest struct {
	Operations []write.WriteOperation `json:"operations"`
	Reason     string                 `json:"reason"`
}

// decodeWriteRequest parses and validates a write request body.
// Returns nil and writes an error response if the request is invalid.
func decodeWriteRequest(w http.ResponseWriter, r *http.Request) *writeRequest {
	r.Body = http.MaxBytesReader(w, r.Body, maxWriteBodySize)
	var body writeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return nil
	}

	if len(body.Operations) == 0 {
		api.WriteError(w, http.StatusBadRequest, "at least one operation is required")
		return nil
	}

	if body.Reason != "" {
		for i := range body.Operations {
			if body.Operations[i].Reason == "" {
				body.Operations[i].Reason = body.Reason
			}
		}
	}

	return &body
}

func handlePreview(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := decodeWriteRequest(w, r)
		if body == nil {
			return
		}

		plan, err := engine.Preview(r.Context(), body.Operations)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

func handleDryRun(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := decodeWriteRequest(w, r)
		if body == nil {
			return
		}

		plan, err := engine.DryRun(r.Context(), body.Operations)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

// planView is the JSON shape returned by handleGetPlan. It mirrors write.Plan
// but omits ApplyToken so that retrieving an existing plan never re-exposes
// the apply credential — the token is only handed out by Preview.
type planView struct {
	ID         string                 `json:"id"`
	CreatedAt  time.Time              `json:"created_at"`
	ExpiresAt  time.Time              `json:"expires_at"`
	Operations []write.WriteOperation `json:"operations"`
	Diffs      []write.PlanDiff       `json:"diffs"`
	SnapshotID int64                  `json:"snapshot_id"`
	Status     string                 `json:"status"`
	Impact     []write.ImpactEntry    `json:"impact,omitempty"`
}

func handleGetPlan(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			api.WriteError(w, http.StatusBadRequest, "plan id is required")
			return
		}

		plan, ok := engine.GetPlan(id)
		if !ok {
			api.WriteError(w, http.StatusNotFound, "plan not found or expired")
			return
		}

		api.WriteJSON(w, http.StatusOK, planView{
			ID:         plan.ID,
			CreatedAt:  plan.CreatedAt,
			ExpiresAt:  plan.ExpiresAt,
			Operations: plan.Operations,
			Diffs:      plan.Diffs,
			SnapshotID: plan.SnapshotID,
			Status:     plan.Status,
			Impact:     plan.Impact,
		})
	}
}

func handleApply(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			api.WriteError(w, http.StatusBadRequest, "plan id is required")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBodySize)
		var body struct {
			ApplyToken string `json:"apply_token"`
			Actor      string `json:"actor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if body.ApplyToken == "" {
			api.WriteError(w, http.StatusBadRequest, "apply_token is required")
			return
		}

		entry, err := engine.Apply(r.Context(), id, body.ApplyToken, body.Actor)
		if err != nil {
			status := http.StatusInternalServerError
			if entry == nil {
				status = http.StatusBadRequest
			}
			api.WriteError(w, status, err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, entry)
	}
}

func handleCancelPlan(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			api.WriteError(w, http.StatusBadRequest, "plan id is required")
			return
		}

		if !engine.CancelPlan(id) {
			api.WriteError(w, http.StatusNotFound, "plan not found or expired")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRollback(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBodySize)
		var body struct {
			SnapshotID int64  `json:"snapshot_id"`
			Actor      string `json:"actor"`
			Reason     string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.SnapshotID == 0 {
			api.WriteError(w, http.StatusBadRequest, "snapshot_id is required")
			return
		}

		plan, err := engine.Rollback(r.Context(), body.SnapshotID, body.Actor, body.Reason)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

func handleAuditLog(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid limit parameter")
				return
			}
			limit = n
		}

		entries, err := engine.QueryAudit(r.Context(), limit)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if entries == nil {
			entries = []write.AuditEntry{}
		}
		api.WriteJSON(w, http.StatusOK, entries)
	}
}

func handleGetAuditEntry(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid audit entry id")
			return
		}

		entry, err := engine.GetAuditEntry(r.Context(), id)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, entry)
	}
}

func handleSchema(engine *write.Engine) http.HandlerFunc {
	// Schema is static at startup — compute once.
	schema := engine.Schema()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		api.WriteJSON(w, http.StatusOK, map[string]any{
			"tables": schema,
		})
	}
}
