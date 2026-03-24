package handler

import (
	"encoding/json"
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/write"
)

// RegisterFailover registers the HA failover, evacuate, and restore endpoints.
func RegisterFailover(mux *http.ServeMux, engine *write.Engine) {
	mux.HandleFunc("POST /api/v1/write/failover", handleFailover(engine))
	mux.HandleFunc("POST /api/v1/write/evacuate", handleEvacuate(engine))
	mux.HandleFunc("POST /api/v1/write/restore", handleRestore(engine))
}

func handleFailover(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBodySize)
		var body struct {
			GroupName     string `json:"group_name"`
			TargetChassis string `json:"target_chassis"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.GroupName == "" {
			api.WriteError(w, http.StatusBadRequest, "group_name is required")
			return
		}
		if body.TargetChassis == "" {
			api.WriteError(w, http.StatusBadRequest, "target_chassis is required")
			return
		}

		plan, err := engine.Failover(r.Context(), body.GroupName, body.TargetChassis)
		if err != nil {
			api.WriteError(w, writeErrorStatus(err), err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

func handleEvacuate(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBodySize)
		var body struct {
			ChassisName string `json:"chassis_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.ChassisName == "" {
			api.WriteError(w, http.StatusBadRequest, "chassis_name is required")
			return
		}

		plan, err := engine.Evacuate(r.Context(), body.ChassisName)
		if err != nil {
			api.WriteError(w, writeErrorStatus(err), err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

func handleRestore(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxWriteBodySize)
		var body struct {
			ChassisName string `json:"chassis_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if body.ChassisName == "" {
			api.WriteError(w, http.StatusBadRequest, "chassis_name is required")
			return
		}

		plan, err := engine.Restore(r.Context(), body.ChassisName)
		if err != nil {
			api.WriteError(w, writeErrorStatus(err), err.Error())
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

// writeErrorStatus returns http.StatusBadRequest for user input errors and
// http.StatusInternalServerError for infrastructure/OVSDB errors.
func writeErrorStatus(err error) int {
	if write.IsInputError(err) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
