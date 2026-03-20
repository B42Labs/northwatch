package handler

import (
	"context"
	"net/http"
	"sort"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// FlowEntry represents a single logical flow in a pipeline table.
type FlowEntry struct {
	UUID     string `json:"uuid"`
	Priority int    `json:"priority"`
	Match    string `json:"match"`
	Actions  string `json:"actions"`
}

// FlowTableGroup groups flows by table_id within a pipeline.
type FlowTableGroup struct {
	TableID int         `json:"table_id"`
	Flows   []FlowEntry `json:"flows"`
}

// FlowPipelineResponse is the JSON response for GET /api/v1/flows.
type FlowPipelineResponse struct {
	DatapathUUID string            `json:"datapath_uuid"`
	DatapathName string            `json:"datapath_name"`
	Ingress      []FlowTableGroup  `json:"ingress"`
	Egress       []FlowTableGroup  `json:"egress"`
}

// RegisterFlows registers the flow pipeline endpoint.
func RegisterFlows(mux *http.ServeMux, sbClient client.Client) {
	mux.HandleFunc("GET /api/v1/flows", handleFlows(sbClient))
}

func handleFlows(sbClient client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		datapathUUID := r.URL.Query().Get("datapath")
		if datapathUUID == "" {
			api.WriteError(w, http.StatusBadRequest, "datapath query parameter is required")
			return
		}

		ctx := r.Context()

		// Fetch flows for the given datapath
		var flows []sb.LogicalFlow
		err := sbClient.WhereCache(func(f *sb.LogicalFlow) bool {
			return f.LogicalDatapath != nil && *f.LogicalDatapath == datapathUUID
		}).List(ctx, &flows)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list logical flows")
			return
		}

		// Resolve datapath name
		datapathName := resolveDatapathName(ctx, sbClient, datapathUUID)

		// Group by pipeline then table_id
		ingressMap := make(map[int][]FlowEntry)
		egressMap := make(map[int][]FlowEntry)

		for _, f := range flows {
			entry := FlowEntry{
				UUID:     f.UUID,
				Priority: f.Priority,
				Match:    f.Match,
				Actions:  f.Actions,
			}
			switch f.Pipeline {
			case "ingress":
				ingressMap[f.TableID] = append(ingressMap[f.TableID], entry)
			case "egress":
				egressMap[f.TableID] = append(egressMap[f.TableID], entry)
			}
		}

		resp := FlowPipelineResponse{
			DatapathUUID: datapathUUID,
			DatapathName: datapathName,
			Ingress:      buildFlowTableGroups(ingressMap),
			Egress:       buildFlowTableGroups(egressMap),
		}

		api.WriteJSON(w, http.StatusOK, resp)
	}
}

func buildFlowTableGroups(m map[int][]FlowEntry) []FlowTableGroup {
	groups := make([]FlowTableGroup, 0, len(m))
	for tableID, flows := range m {
		// Sort flows by priority descending
		sort.Slice(flows, func(i, j int) bool {
			return flows[i].Priority > flows[j].Priority
		})
		groups = append(groups, FlowTableGroup{
			TableID: tableID,
			Flows:   flows,
		})
	}
	// Sort groups by table_id ascending
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].TableID < groups[j].TableID
	})
	return groups
}

func resolveDatapathName(ctx context.Context, sbClient client.Client, datapathUUID string) string {
	var datapaths []sb.DatapathBinding
	err := sbClient.WhereCache(func(dp *sb.DatapathBinding) bool {
		return dp.UUID == datapathUUID
	}).List(ctx, &datapaths)
	if err != nil || len(datapaths) == 0 {
		return ""
	}

	dp := datapaths[0]
	if name, ok := dp.ExternalIDs["logical-switch"]; ok {
		return name
	}
	if name, ok := dp.ExternalIDs["logical-router"]; ok {
		return name
	}
	return ""
}
