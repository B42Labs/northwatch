package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/write"
)

// writeEngineError maps a write.Engine error to an HTTP status: rate limiting to
// 429, stale-state conflicts to 409, user input errors to 400, and everything
// else (infrastructure failures) to 500.
func writeEngineError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, write.ErrRateLimited):
		api.WriteError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, write.ErrStaleState):
		api.WriteError(w, http.StatusConflict, err.Error())
	case write.IsInputError(err):
		api.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		api.WriteInternalError(w, r, err)
	}
}

// applyEngineError maps an Apply error to an HTTP status. Apply differs from the
// preview-time endpoints in two ways that this taxonomy captures: it returns an
// audit entry (a non-nil entry means apply was reached and recorded a failure),
// and a fresh input error means the plan validated clean at preview but the
// referenced state changed since — a retryable conflict (409), not a bad
// request (400).
func applyEngineError(w http.ResponseWriter, r *http.Request, entry *write.AuditEntry, err error) {
	switch {
	case errors.Is(err, write.ErrRateLimited):
		api.WriteError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, write.ErrStaleState):
		api.WriteError(w, http.StatusConflict, err.Error())
	case write.IsInputError(err):
		// Re-validation at apply failed even though preview validated clean:
		// the database changed since preview (e.g. a referenced row was
		// deleted). This is a retryable conflict, not a malformed request.
		api.WriteError(w, http.StatusConflict, err.Error())
	case entry == nil:
		// No audit entry means apply was never reached (bad/expired plan,
		// invalid token): a client error.
		api.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		api.WriteInternalError(w, r, err)
	}
}

// auditActor derives the write-audit actor from the credential that authorized
// the request, so an audit entry names whoever presented the token rather than
// whatever string the client typed into the request body. It falls back to
// "anonymous" only when the deployment runs with --insecure-no-auth, where no
// credential exists to attribute the change to.
func auditActor(r *http.Request) string {
	if actor := api.ActorFromContext(r.Context()); actor != "" {
		return actor
	}
	return "anonymous"
}

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
	var body writeRequest
	if !decodeJSONBody(w, r, &body, maxBodySize, false) {
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
			writeEngineError(w, r, err)
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
			writeEngineError(w, r, err)
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
	Warnings   []string               `json:"warnings,omitempty"`
}

func handleGetPlan(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

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
			Warnings:   plan.Warnings,
		})
	}
}

func handleApply(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var body struct {
			ApplyToken string `json:"apply_token"`
		}
		if !decodeJSONBody(w, r, &body, maxBodySize, false) {
			return
		}

		if body.ApplyToken == "" {
			api.WriteError(w, http.StatusBadRequest, "apply_token is required")
			return
		}

		entry, err := engine.Apply(r.Context(), id, body.ApplyToken, auditActor(r))
		if err != nil {
			applyEngineError(w, r, entry, err)
			return
		}

		api.WriteJSON(w, http.StatusOK, entry)
	}
}

func handleCancelPlan(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		if !engine.CancelPlan(id) {
			api.WriteError(w, http.StatusNotFound, "plan not found or expired")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRollback(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SnapshotID int64  `json:"snapshot_id"`
			Reason     string `json:"reason"`
		}
		if !decodeJSONBody(w, r, &body, maxBodySize, false) {
			return
		}
		if body.SnapshotID == 0 {
			api.WriteError(w, http.StatusBadRequest, "snapshot_id is required")
			return
		}

		plan, err := engine.Rollback(r.Context(), body.SnapshotID, auditActor(r), body.Reason)
		if err != nil {
			writeEngineError(w, r, err)
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
			api.WriteInternalError(w, r, err)
			return
		}
		api.WriteJSONList(w, http.StatusOK, entries)
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
