package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/cluster"
)

// RegisterClusters registers the clusters listing endpoint.
func RegisterClusters(mux *http.ServeMux, reg *cluster.Registry) {
	mux.HandleFunc("GET /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		type clusterInfo struct {
			Name  string `json:"name"`
			Label string `json:"label"`
			Ready bool   `json:"ready"`
		}
		var list []clusterInfo
		for _, c := range reg.List() {
			list = append(list, clusterInfo{
				Name:  c.Name,
				Label: c.Label,
				Ready: c.DBs.Ready(),
			})
		}
		if list == nil {
			list = []clusterInfo{}
		}
		api.WriteJSON(w, http.StatusOK, map[string]any{"clusters": list})
	})
}
