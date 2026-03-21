package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscapeXML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"plain", "hello", "hello"},
		{"ampersand", "a&b", "a&amp;b"},
		{"less than", "a<b", "a&lt;b"},
		{"greater than", "a>b", "a&gt;b"},
		{"single quote", "it's", "it&apos;s"},
		{"double quote", `say "hi"`, "say &quot;hi&quot;"},
		{"all special", `<a>&'b"`, "&lt;a&gt;&amp;&apos;b&quot;"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, escapeXML(tc.input))
		})
	}
}

func TestRenderTopologySVG_Empty(t *testing.T) {
	svg := renderTopologySVG(TopologyResponse{})
	assert.Contains(t, svg, "No topology data")
	assert.Contains(t, svg, "<svg")
}

func TestRenderTopologySVG_WithNodes(t *testing.T) {
	topo := TopologyResponse{
		Nodes: []TopologyNode{
			{ID: "r1", Type: "router", Label: "router0"},
			{ID: "s1", Type: "switch", Label: "switch0"},
			{ID: "c1", Type: "chassis", Label: "chassis0"},
		},
		Edges: []TopologyEdge{
			{Source: "r1", Target: "s1", Type: "patch"},
			{Source: "s1", Target: "c1", Type: "binding"},
		},
	}

	svg := renderTopologySVG(topo)
	assert.Contains(t, svg, "<svg")
	assert.Contains(t, svg, "</svg>")
	assert.Contains(t, svg, "router0")
	assert.Contains(t, svg, "switch0")
	assert.Contains(t, svg, "chassis0")
	assert.Contains(t, svg, "Routers")
	assert.Contains(t, svg, "Switches")
	assert.Contains(t, svg, "Chassis")
	assert.Contains(t, svg, "node-router")
	assert.Contains(t, svg, "node-switch")
	assert.Contains(t, svg, "node-chassis")
	assert.Contains(t, svg, "edge-binding")
}

func TestRenderTopologySVG_LongLabel(t *testing.T) {
	topo := TopologyResponse{
		Nodes: []TopologyNode{
			{ID: "s1", Type: "switch", Label: "very-long-switch-name-here"},
		},
	}

	svg := renderTopologySVG(topo)
	assert.Contains(t, svg, "very-long-switch...")
	assert.NotContains(t, svg, "very-long-switch-name-here")
}

func TestRenderTopologySVG_XMLEscaping(t *testing.T) {
	topo := TopologyResponse{
		Nodes: []TopologyNode{
			{ID: "s1", Type: "switch", Label: "sw<0>&test"},
		},
	}

	svg := renderTopologySVG(topo)
	assert.Contains(t, svg, "sw&lt;0&gt;&amp;test")
}

func TestRenderTopologySVG_SkipsMissingEdgePositions(t *testing.T) {
	topo := TopologyResponse{
		Nodes: []TopologyNode{
			{ID: "s1", Type: "switch", Label: "sw0"},
		},
		Edges: []TopologyEdge{
			{Source: "s1", Target: "missing", Type: "patch"},
		},
	}

	svg := renderTopologySVG(topo)
	assert.Contains(t, svg, "<svg")
	assert.NotContains(t, svg, "<path")
}

func TestHandleListTraces(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	store.Store("t1", TraceResponse{PortName: "p1"})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/debug/traces", handleListTraces(store))

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/debug/traces", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var traces []StoredTrace
	err = json.NewDecoder(w.Body).Decode(&traces)
	require.NoError(t, err)
	assert.Len(t, traces, 1)
}

func TestHandleListTraces_Empty(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/debug/traces", handleListTraces(store))

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/debug/traces", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", strings.TrimSpace(w.Body.String()))
}

func TestHandleExportTrace_NotFound(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export/trace/{id}", handleExportTrace(store))

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/export/trace/nonexistent", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleExportTrace_JSON(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	store.Store("abc123", TraceResponse{
		PortName:     "lsp0",
		DatapathName: "sw0",
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export/trace/{id}", handleExportTrace(store))

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/export/trace/abc123", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "trace-abc123.json")
}

func TestHandleExportTrace_Text(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	store.Store("abc123", TraceResponse{
		PortUUID:     "port-uuid",
		PortName:     "lsp0",
		DatapathUUID: "dp-uuid",
		DatapathName: "sw0",
		DstIP:        "10.0.0.1",
		Protocol:     "tcp",
		Stages: []TraceStage{
			{
				Pipeline:  "ingress",
				TableID:   0,
				TableName: "ls_in_port_sec_l2",
				Flows: []TraceFlowEntry{
					{Priority: 50, Match: "1", Actions: "next;", Hint: "default", Selected: true},
					{Priority: 0, Match: "1", Actions: "drop;", Hint: "", Selected: false},
				},
			},
		},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export/trace/{id}", handleExportTrace(store))

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/export/trace/abc123?format=text", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Packet Trace: abc123")
	assert.Contains(t, body, "Port: lsp0 (port-uuid)")
	assert.Contains(t, body, "Datapath: sw0 (dp-uuid)")
	assert.Contains(t, body, "Destination IP: 10.0.0.1")
	assert.Contains(t, body, "Protocol: tcp")
	assert.Contains(t, body, "[ingress] Table 0: ls_in_port_sec_l2")
	assert.Contains(t, body, "* priority=50")
}

func TestHandleExportTrace_InvalidFormat(t *testing.T) {
	store := NewTraceStore(5 * time.Minute)
	store.Store("abc123", TraceResponse{PortName: "lsp0"})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export/trace/{id}", handleExportTrace(store))

	req, err := http.NewRequestWithContext(context.Background(), "GET", "/api/v1/export/trace/abc123?format=xml", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
