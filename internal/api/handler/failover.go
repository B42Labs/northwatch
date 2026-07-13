package handler

import (
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
		var body struct {
			GroupName     string `json:"group_name"`
			TargetChassis string `json:"target_chassis"`
		}
		if !decodeJSONBody(w, r, &body, maxBodySize, false) {
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
			writeFailoverError(w, r, err)
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

func handleEvacuate(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChassisName string `json:"chassis_name"`
		}
		if !decodeJSONBody(w, r, &body, maxBodySize, false) {
			return
		}
		if body.ChassisName == "" {
			api.WriteError(w, http.StatusBadRequest, "chassis_name is required")
			return
		}

		plan, err := engine.Evacuate(r.Context(), body.ChassisName)
		if err != nil {
			writeFailoverError(w, r, err)
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

func handleRestore(engine *write.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChassisName string `json:"chassis_name"`
		}
		if !decodeJSONBody(w, r, &body, maxBodySize, false) {
			return
		}
		if body.ChassisName == "" {
			api.WriteError(w, http.StatusBadRequest, "chassis_name is required")
			return
		}

		plan, err := engine.Restore(r.Context(), body.ChassisName)
		if err != nil {
			writeFailoverError(w, r, err)
			return
		}

		api.WriteJSON(w, http.StatusOK, plan)
	}
}

// writeFailoverError answers 400 with the message for user input errors, and
// routes everything else (infrastructure/OVSDB failures) through the shared 500
// policy: logged server-side, generic to the client.
func writeFailoverError(w http.ResponseWriter, r *http.Request, err error) {
	if write.IsInputError(err) {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteInternalError(w, r, err)
}
