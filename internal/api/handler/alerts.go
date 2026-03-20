package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/b42labs/northwatch/internal/alert"
	"github.com/b42labs/northwatch/internal/api"
)

// RegisterAlerts registers alert REST endpoints.
func RegisterAlerts(mux *http.ServeMux, engine *alert.Engine) {
	mux.HandleFunc("GET /api/v1/alerts", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, engine.ActiveAlerts())
	})

	mux.HandleFunc("GET /api/v1/alerts/rules", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, http.StatusOK, engine.Rules())
	})

	mux.HandleFunc("PUT /api/v1/alerts/rules/{name}", handleSetRuleEnabled(engine))
	mux.HandleFunc("GET /api/v1/alerts/silences", handleListSilences(engine))
	mux.HandleFunc("POST /api/v1/alerts/silences", handleCreateSilence(engine))
	mux.HandleFunc("DELETE /api/v1/alerts/silences/{id}", handleDeleteSilence(engine))
}

func handleSetRuleEnabled(engine *alert.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			api.WriteError(w, http.StatusBadRequest, "rule name is required")
			return
		}

		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := engine.SetRuleEnabled(name, body.Enabled); err != nil {
			api.WriteError(w, http.StatusNotFound, err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, map[string]any{
			"name":    name,
			"enabled": body.Enabled,
		})
	}
}

func handleListSilences(engine *alert.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		silences := engine.ListSilences()
		if silences == nil {
			silences = []alert.Silence{}
		}
		api.WriteJSON(w, http.StatusOK, silences)
	}
}

func handleCreateSilence(engine *alert.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Rule      string            `json:"rule"`
			Matchers  map[string]string `json:"matchers"`
			Duration  string            `json:"duration"`
			Comment   string            `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if body.Rule == "" && len(body.Matchers) == 0 {
			api.WriteError(w, http.StatusBadRequest, "at least one of 'rule' or 'matchers' is required")
			return
		}

		duration := 1 * time.Hour // default
		if body.Duration != "" {
			d, err := time.ParseDuration(body.Duration)
			if err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid duration format")
				return
			}
			duration = d
		}

		now := time.Now().UTC()
		s := alert.Silence{
			CreatedAt: now,
			ExpiresAt: now.Add(duration),
			Rule:      body.Rule,
			Matchers:  body.Matchers,
			Comment:   body.Comment,
		}

		id := engine.AddSilence(s)
		s.ID = id

		api.WriteJSON(w, http.StatusCreated, s)
	}
}

func handleDeleteSilence(engine *alert.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			api.WriteError(w, http.StatusBadRequest, "silence id is required")
			return
		}

		if err := engine.RemoveSilence(id); err != nil {
			api.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
