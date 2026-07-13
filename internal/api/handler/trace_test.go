package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/b42labs/northwatch/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleTrace_HTTP(t *testing.T) {
	sbc := testutil.SetupSBTestClient(t)

	dpUUID := insertDatapath(t, sbc, map[string]string{"logical-switch": "sw-trace"})
	pbUUID := insertPortBindingOn(t, sbc, "lsp-trace", dpUUID)
	insertLogicalFlow(t, sbc, dpUUID, "ingress", 0, 100, `inport == "lsp-trace"`, "next;", map[string]string{"stage-name": "ls_in_port_sec"})
	insertLogicalFlow(t, sbc, dpUUID, "egress", 0, 0, "1", "drop;", nil)

	store := NewTraceStore(5 * time.Minute)
	mux := http.NewServeMux()
	RegisterTrace(mux, sbc, store)

	t.Run("happy path stores trace", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
			"/api/v1/debug/trace?port="+pbUUID+"&dst_ip=10.0.0.1&protocol=tcp", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp TraceResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "lsp-trace", resp.PortName)
		assert.Equal(t, "sw-trace", resp.DatapathName)
		assert.Equal(t, "10.0.0.1", resp.DstIP)
		assert.Equal(t, "tcp", resp.Protocol)
		require.NotEmpty(t, resp.ID)
		require.NotEmpty(t, resp.Stages)

		// The generated trace is retained for later export.
		stored, ok := store.Get(resp.ID)
		require.True(t, ok)
		assert.Equal(t, "lsp-trace", stored.Trace.PortName)
	})

	t.Run("missing port param", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/debug/trace", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("port not found", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/debug/trace?port=missing", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestGenerateTraceID_Unique(t *testing.T) {
	a := generateTraceID()
	b := generateTraceID()
	assert.Len(t, a, 16)
	assert.NotEqual(t, a, b)
}

func TestClassifyFlowMatch(t *testing.T) {
	tests := []struct {
		name     string
		match    string
		priority int
		port     string
		dstIP    string
		protocol string
		want     string
	}{
		{
			name:     "default priority 0",
			match:    "1",
			priority: 0,
			port:     "port1",
			want:     "default",
		},
		{
			name:     "likely: port name in match",
			match:    `inport == "port1" && ip4`,
			priority: 100,
			port:     "port1",
			want:     "likely",
		},
		{
			name:     "possible: dst_ip in match",
			match:    `ip4.dst == 10.0.0.1`,
			priority: 100,
			port:     "port1",
			dstIP:    "10.0.0.1",
			want:     "possible",
		},
		{
			name:     "possible: protocol in match",
			match:    `ip4 && tcp`,
			priority: 100,
			port:     "port1",
			protocol: "tcp",
			want:     "possible",
		},
		{
			name:     "possible: match is 1 (any)",
			match:    "1",
			priority: 100,
			port:     "port1",
			want:     "possible",
		},
		{
			name:     "no match: unrelated flow",
			match:    `inport == "other-port" && ip4.dst == 192.168.1.1`,
			priority: 100,
			port:     "port1",
			dstIP:    "10.0.0.1",
			want:     "",
		},
		{
			name:     "empty port name",
			match:    `inport == "port1"`,
			priority: 100,
			port:     "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyFlowMatch(tt.match, tt.priority, tt.port, tt.dstIP, tt.protocol)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildTraceStages(t *testing.T) {
	dp := "dp-1"
	flows := []sb.LogicalFlow{
		{
			UUID:            "f1",
			Pipeline:        "ingress",
			TableID:         0,
			Priority:        100,
			Match:           `inport == "port1"`,
			Actions:         "next;",
			LogicalDatapath: &dp,
			ExternalIDs:     map[string]string{"stage-name": "ls_in_check_port_sec"},
		},
		{
			UUID:            "f2",
			Pipeline:        "ingress",
			TableID:         0,
			Priority:        0,
			Match:           "1",
			Actions:         "drop;",
			LogicalDatapath: &dp,
			ExternalIDs:     map[string]string{"stage-name": "ls_in_check_port_sec"},
		},
		{
			UUID:            "f3",
			Pipeline:        "ingress",
			TableID:         8,
			Priority:        65535,
			Match:           `inport == "port1" && ip4`,
			Actions:         "next;",
			LogicalDatapath: &dp,
		},
		{
			UUID:            "f4",
			Pipeline:        "egress",
			TableID:         0,
			Priority:        100,
			Match:           "ip4",
			Actions:         "output;",
			LogicalDatapath: &dp,
		},
	}

	stages := buildTraceStages(flows, "port1", "", "")

	// Should be sorted: ingress first, then by table_id
	require.GreaterOrEqual(t, len(stages), 2)

	// First stage should be ingress table 0
	assert.Equal(t, "ingress", stages[0].Pipeline)
	assert.Equal(t, 0, stages[0].TableID)
	assert.Equal(t, "ls_in_check_port_sec", stages[0].TableName)

	// Check that flows are sorted by priority descending
	require.Len(t, stages[0].Flows, 2)
	assert.Equal(t, 100, stages[0].Flows[0].Priority)
	assert.Equal(t, 0, stages[0].Flows[1].Priority)

	// Check hints
	assert.Equal(t, "likely", stages[0].Flows[0].Hint)  // has port name
	assert.Equal(t, "default", stages[0].Flows[1].Hint) // priority 0

	// Check selection — "likely" flow should be selected
	assert.True(t, stages[0].Flows[0].Selected)
	assert.False(t, stages[0].Flows[1].Selected)

	// Egress should come after all ingress
	lastStage := stages[len(stages)-1]
	assert.Equal(t, "egress", lastStage.Pipeline)
}

func TestSelectBestMatch(t *testing.T) {
	tests := []struct {
		name      string
		hints     []string
		wantIndex int
	}{
		{
			name:      "likely wins",
			hints:     []string{"likely", "possible", "default"},
			wantIndex: 0,
		},
		{
			name:      "possible over default",
			hints:     []string{"", "possible", "default"},
			wantIndex: 1,
		},
		{
			name:      "default as fallback",
			hints:     []string{"", "", "default"},
			wantIndex: 2,
		},
		{
			name:      "all empty",
			hints:     []string{"", ""},
			wantIndex: -1, // no selection
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flows := make([]TraceFlowEntry, len(tt.hints))
			for i, h := range tt.hints {
				flows[i] = TraceFlowEntry{
					UUID:     "f" + string(rune('0'+i)),
					Priority: 100 - i,
					Hint:     h,
				}
			}

			selectBestMatch(flows)

			selectedIdx := -1
			for i, f := range flows {
				if f.Selected {
					selectedIdx = i
				}
			}
			assert.Equal(t, tt.wantIndex, selectedIdx)
		})
	}
}

// TestHandleTrace_CacheErrorIsNot404 pins the split between a broken cache and a
// port that does not exist. A disconnected client fails the lookup; reporting
// that as 404 would tell the caller the port is absent, which is a different
// problem than the one that actually occurred.
func TestHandleTrace_CacheErrorIsNot404(t *testing.T) {
	mux := http.NewServeMux()
	RegisterTrace(mux, errListClient{err: errors.New("cache read failed")}, NewTraceStore(5*time.Minute))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/debug/trace?port=00000000-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "internal server error")
}

func TestHandleTrace_UnknownPortIs404(t *testing.T) {
	sbc := testutil.SetupSBTestClient(t)

	mux := http.NewServeMux()
	RegisterTrace(mux, sbc, NewTraceStore(5*time.Minute))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/debug/trace?port=00000000-0000-0000-0000-000000000000", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "port binding not found")
}
