package handler

import (
	"net/http"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/cluster"
)

// RegisterClusters registers the clusters listing endpoint.
func RegisterClusters(mux *http.ServeMux, reg *cluster.Registry) {
	mux.HandleFunc("GET /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		type snapshotInfo struct {
			SourceID  int64  `json:"sourceId"`
			CreatedAt string `json:"createdAt,omitempty"`
			NBAddr    string `json:"nbAddr,omitempty"`
			SBAddr    string `json:"sbAddr,omitempty"`
		}
		type clusterInfo struct {
			Name     string        `json:"name"`
			Label    string        `json:"label"`
			Ready    bool          `json:"ready"`
			Mode     string        `json:"mode"`
			Snapshot *snapshotInfo `json:"snapshot,omitempty"`
		}
		var list []clusterInfo
		for _, c := range reg.List() {
			mode := c.Mode
			if mode == "" {
				mode = "live"
			}
			ci := clusterInfo{
				Name:  c.Name,
				Label: c.Label,
				Ready: c.DBs.Ready(),
				Mode:  mode,
			}
			if c.Snapshot != nil {
				ci.Snapshot = &snapshotInfo{
					SourceID:  c.Snapshot.SourceID,
					CreatedAt: c.Snapshot.CreatedAt,
					NBAddr:    c.Snapshot.NBAddr,
					SBAddr:    c.Snapshot.SBAddr,
				}
			}
			list = append(list, ci)
		}
		if list == nil {
			list = []clusterInfo{}
		}
		api.WriteJSON(w, http.StatusOK, map[string]any{"clusters": list})
	})
}
