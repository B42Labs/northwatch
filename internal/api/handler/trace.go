package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// TraceFlowEntry is a flow with heuristic match classification.
type TraceFlowEntry struct {
	UUID     string `json:"uuid"`
	Priority int    `json:"priority"`
	Match    string `json:"match"`
	Actions  string `json:"actions"`
	Hint     string `json:"hint"`     // "likely", "possible", "default", ""
	Selected bool   `json:"selected"` // best-match in this table
}

// TraceStage represents a single table in the traced pipeline.
type TraceStage struct {
	Pipeline  string           `json:"pipeline"`
	TableID   int              `json:"table_id"`
	TableName string           `json:"table_name,omitempty"`
	Flows     []TraceFlowEntry `json:"flows"`
}

// TraceResponse is the full trace result.
type TraceResponse struct {
	ID           string       `json:"id,omitempty"`
	PortUUID     string       `json:"port_uuid"`
	PortName     string       `json:"port_name"`
	DatapathUUID string       `json:"datapath_uuid"`
	DatapathName string       `json:"datapath_name"`
	DstIP        string       `json:"dst_ip,omitempty"`
	Protocol     string       `json:"protocol,omitempty"`
	Stages       []TraceStage `json:"stages"`
}

// RegisterTrace registers the packet trace endpoint.
func RegisterTrace(mux *http.ServeMux, sbClient client.Client, traceStore *TraceStore) {
	mux.HandleFunc("GET /api/v1/debug/trace", handleTrace(sbClient, traceStore))
}

func generateTraceID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func handleTrace(sbClient client.Client, traceStore *TraceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		portUUID := r.URL.Query().Get("port")
		if portUUID == "" {
			api.WriteError(w, http.StatusBadRequest, "port query parameter is required")
			return
		}

		dstIP := r.URL.Query().Get("dst_ip")
		protocol := r.URL.Query().Get("protocol")
		ctx := r.Context()

		// Resolve port binding
		var pbs []sb.PortBinding
		err := sbClient.WhereCache(func(pb *sb.PortBinding) bool {
			return pb.UUID == portUUID
		}).List(ctx, &pbs)
		if err != nil || len(pbs) == 0 {
			api.WriteError(w, http.StatusNotFound, "port binding not found")
			return
		}
		pb := pbs[0]
		portName := pb.LogicalPort
		datapathUUID := pb.Datapath

		datapathName := resolveDatapathName(ctx, sbClient, datapathUUID)

		// Fetch all flows for this datapath
		var flows []sb.LogicalFlow
		err = sbClient.WhereCache(func(f *sb.LogicalFlow) bool {
			return f.LogicalDatapath != nil && *f.LogicalDatapath == datapathUUID
		}).List(ctx, &flows)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list logical flows")
			return
		}

		stages := buildTraceStages(flows, portName, dstIP, protocol)

		traceID := generateTraceID()
		resp := TraceResponse{
			ID:           traceID,
			PortUUID:     portUUID,
			PortName:     portName,
			DatapathUUID: datapathUUID,
			DatapathName: datapathName,
			DstIP:        dstIP,
			Protocol:     protocol,
			Stages:       stages,
		}

		if traceStore != nil {
			traceStore.Store(traceID, resp)
		}

		api.WriteJSON(w, http.StatusOK, resp)
	}
}

func buildTraceStages(flows []sb.LogicalFlow, portName, dstIP, protocol string) []TraceStage {
	// Group by (pipeline, table_id)
	type stageKey struct {
		pipeline string
		tableID  int
	}
	grouped := make(map[stageKey][]sb.LogicalFlow)
	tableNames := make(map[stageKey]string)

	for _, f := range flows {
		key := stageKey{f.Pipeline, f.TableID}
		grouped[key] = append(grouped[key], f)
		if name, ok := f.ExternalIDs["stage-name"]; ok && tableNames[key] == "" {
			tableNames[key] = name
		}
	}

	// Build stages
	var stages []TraceStage
	for key, stageFlows := range grouped {
		tableName := tableNames[key]
		if tableName == "" {
			tableName = OVNTableName(key.tableID)
		}

		traceFlows := make([]TraceFlowEntry, 0, len(stageFlows))
		for _, f := range stageFlows {
			hint := classifyFlowMatch(f.Match, f.Priority, portName, dstIP, protocol)
			traceFlows = append(traceFlows, TraceFlowEntry{
				UUID:     f.UUID,
				Priority: f.Priority,
				Match:    f.Match,
				Actions:  f.Actions,
				Hint:     hint,
			})
		}

		// Sort by priority descending
		sort.Slice(traceFlows, func(i, j int) bool {
			return traceFlows[i].Priority > traceFlows[j].Priority
		})

		// Select best match per table
		selectBestMatch(traceFlows)

		stages = append(stages, TraceStage{
			Pipeline:  key.pipeline,
			TableID:   key.tableID,
			TableName: tableName,
			Flows:     traceFlows,
		})
	}

	// Sort stages: ingress first, then by table_id
	sort.Slice(stages, func(i, j int) bool {
		if stages[i].Pipeline != stages[j].Pipeline {
			return stages[i].Pipeline == "ingress"
		}
		return stages[i].TableID < stages[j].TableID
	})

	return stages
}

// classifyFlowMatch applies heuristic matching to determine how likely a flow
// matches the given packet parameters.
func classifyFlowMatch(match string, priority int, portName, dstIP, protocol string) string {
	if priority == 0 {
		return "default"
	}

	matchLower := strings.ToLower(match)

	// Check for port name in match expression
	hasPort := portName != "" && strings.Contains(matchLower, strings.ToLower(portName))

	// Check for destination IP
	hasDstIP := dstIP != "" && strings.Contains(match, dstIP)

	// Check for protocol
	hasProtocol := false
	if protocol != "" {
		protocolLower := strings.ToLower(protocol)
		hasProtocol = strings.Contains(matchLower, protocolLower)
	}

	if hasPort {
		return "likely"
	}
	if hasDstIP || hasProtocol {
		return "possible"
	}
	if match == "1" {
		return "possible"
	}
	return ""
}

func selectBestMatch(flows []TraceFlowEntry) {
	// Select highest-priority flow with best hint
	bestIdx := -1
	bestScore := 0 // require at least score 1 (default) to select

	for i := range flows {
		score := hintScore(flows[i].Hint)
		// Higher priority (earlier in slice since sorted descending) wins on tie
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestIdx >= 0 {
		flows[bestIdx].Selected = true
	}
}

func hintScore(hint string) int {
	switch hint {
	case "likely":
		return 3
	case "possible":
		return 2
	case "default":
		return 1
	default:
		return 0
	}
}
