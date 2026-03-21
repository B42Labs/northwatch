package handler

import (
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// RegisterExport registers export endpoints.
func RegisterExport(mux *http.ServeMux, nbClient, sbClient client.Client, traceStore *TraceStore) {
	mux.HandleFunc("GET /api/v1/export/topology", handleExportTopology(nbClient, sbClient))
	mux.HandleFunc("GET /api/v1/export/trace/{id}", handleExportTrace(traceStore))
	mux.HandleFunc("GET /api/v1/debug/traces", handleListTraces(traceStore))
}

func handleExportTopology(nbClient, sbClient client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "svg"
		}
		if format != "svg" && format != "json" {
			api.WriteError(w, http.StatusBadRequest, "format must be 'svg' or 'json'")
			return
		}

		var switches []nb.LogicalSwitch
		if err := nbClient.List(ctx, &switches); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list logical switches")
			return
		}
		var routers []nb.LogicalRouter
		if err := nbClient.List(ctx, &routers); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list logical routers")
			return
		}
		var lsps []nb.LogicalSwitchPort
		if err := nbClient.List(ctx, &lsps); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list logical switch ports")
			return
		}
		var lrps []nb.LogicalRouterPort
		if err := nbClient.List(ctx, &lrps); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list logical router ports")
			return
		}
		var chassisList []sb.Chassis
		if err := sbClient.List(ctx, &chassisList); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list chassis")
			return
		}
		var portBindings []sb.PortBinding
		if err := sbClient.List(ctx, &portBindings); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list port bindings")
			return
		}
		var datapaths []sb.DatapathBinding
		if err := sbClient.List(ctx, &datapaths); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to list datapath bindings")
			return
		}

		topo := buildTopology(switches, routers, lsps, lrps, chassisList, portBindings, datapaths, false)

		if format == "json" {
			w.Header().Set("Content-Disposition", "attachment; filename=topology.json")
			api.WriteJSON(w, http.StatusOK, topo)
			return
		}

		svg := renderTopologySVG(topo)
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Content-Disposition", "attachment; filename=topology.svg")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, svg)
	}
}

func renderTopologySVG(topo TopologyResponse) string {
	if len(topo.Nodes) == 0 {
		return `<svg xmlns="http://www.w3.org/2000/svg" width="400" height="200"><text x="200" y="100" text-anchor="middle" font-family="sans-serif" font-size="14" fill="#666">No topology data</text></svg>`
	}

	const (
		nodeWidth  = 140
		nodeHeight = 50
		paddingX   = 40
		paddingY   = 80
		marginX    = 60
		marginY    = 60
	)

	var routerNodes, switchNodes, chassisNodes []TopologyNode
	for _, n := range topo.Nodes {
		switch n.Type {
		case "router":
			routerNodes = append(routerNodes, n)
		case "switch":
			switchNodes = append(switchNodes, n)
		case "chassis":
			chassisNodes = append(chassisNodes, n)
		}
	}

	type nodePos struct{ X, Y float64 }
	positions := make(map[string]nodePos)

	layoutRow := func(nodes []TopologyNode, y float64) {
		for i, n := range nodes {
			positions[n.ID] = nodePos{
				X: float64(marginX + i*(nodeWidth+paddingX)),
				Y: y,
			}
		}
	}

	layoutRow(routerNodes, float64(marginY))
	layoutRow(switchNodes, float64(marginY+nodeHeight+paddingY))
	layoutRow(chassisNodes, float64(marginY+2*(nodeHeight+paddingY)))

	maxX := float64(marginX)
	maxY := float64(marginY + 3*(nodeHeight+paddingY))
	for _, p := range positions {
		if p.X+float64(nodeWidth+marginX) > maxX {
			maxX = p.X + float64(nodeWidth+marginX)
		}
	}
	if maxX < 400 {
		maxX = 400
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, maxX, maxY, maxX, maxY)
	b.WriteString("\n")
	b.WriteString(`<defs><style>
.node-router{fill:#4A90D9;stroke:#2C5F8A;stroke-width:2;rx:8}
.node-switch{fill:#50B86C;stroke:#2E7D42;stroke-width:2;rx:4}
.node-chassis{fill:#E8913A;stroke:#B06B1E;stroke-width:2;rx:4}
.label{font-family:sans-serif;font-size:12px;fill:#fff;text-anchor:middle;dominant-baseline:central}
.edge{stroke:#999;stroke-width:1.5;fill:none}
.edge-binding{stroke:#CCC;stroke-width:1;stroke-dasharray:4,4;fill:none}
.title{font-family:sans-serif;font-size:11px;fill:#666}
</style></defs>`)
	b.WriteString("\n")
	fmt.Fprintf(&b, `<rect width="%.0f" height="%.0f" fill="#FAFAFA"/>`, maxX, maxY)
	b.WriteString("\n")

	if len(routerNodes) > 0 {
		fmt.Fprintf(&b, `<text x="10" y="%.0f" class="title">Routers</text>`, float64(marginY)-10)
		b.WriteString("\n")
	}
	if len(switchNodes) > 0 {
		fmt.Fprintf(&b, `<text x="10" y="%.0f" class="title">Switches</text>`, float64(marginY+nodeHeight+paddingY)-10)
		b.WriteString("\n")
	}
	if len(chassisNodes) > 0 {
		fmt.Fprintf(&b, `<text x="10" y="%.0f" class="title">Chassis</text>`, float64(marginY+2*(nodeHeight+paddingY))-10)
		b.WriteString("\n")
	}

	// Edges
	for _, e := range topo.Edges {
		from, okFrom := positions[e.Source]
		to, okTo := positions[e.Target]
		if !okFrom || !okTo {
			continue
		}
		class := "edge"
		if e.Type == "binding" {
			class = "edge-binding"
		}
		fromCX := from.X + float64(nodeWidth)/2
		fromCY := from.Y + float64(nodeHeight)/2
		toCX := to.X + float64(nodeWidth)/2
		toCY := to.Y + float64(nodeHeight)/2

		midX := (fromCX + toCX) / 2
		midY := (fromCY + toCY) / 2
		dx := toCX - fromCX
		dy := toCY - fromCY
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist == 0 {
			continue
		}
		offset := dist * 0.1
		ctrlX := midX - (dy/dist)*offset
		ctrlY := midY + (dx/dist)*offset

		fmt.Fprintf(&b, `<path d="M%.1f,%.1f Q%.1f,%.1f %.1f,%.1f" class="%s"/>`,
			fromCX, fromCY, ctrlX, ctrlY, toCX, toCY, class)
		b.WriteString("\n")
	}

	// Nodes
	for _, n := range topo.Nodes {
		pos, ok := positions[n.ID]
		if !ok {
			continue
		}
		class := "node-switch"
		switch n.Type {
		case "router":
			class = "node-router"
		case "chassis":
			class = "node-chassis"
		}

		fmt.Fprintf(&b, `<rect x="%.0f" y="%.0f" width="%d" height="%d" class="%s"/>`,
			pos.X, pos.Y, nodeWidth, nodeHeight, class)
		b.WriteString("\n")

		label := n.Label
		if len(label) > 16 {
			label = label[:16] + "..."
		}
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" class="label">%s</text>`,
			pos.X+float64(nodeWidth)/2, pos.Y+float64(nodeHeight)/2, escapeXML(label))
		b.WriteString("\n")
	}

	b.WriteString("</svg>")
	return b.String()
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func handleExportTrace(store *TraceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			api.WriteError(w, http.StatusBadRequest, "trace id is required")
			return
		}

		trace, ok := store.Get(id)
		if !ok {
			api.WriteError(w, http.StatusNotFound, "trace not found or expired")
			return
		}

		format := r.URL.Query().Get("format")
		if format == "" {
			format = "json"
		}

		switch format {
		case "json":
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=trace-%s.json", id))
			api.WriteJSON(w, http.StatusOK, trace)
		case "text":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=trace-%s.txt", id))
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, "Packet Trace: %s\n", trace.ID)
			_, _ = fmt.Fprintf(w, "Port: %s (%s)\n", trace.Trace.PortName, trace.Trace.PortUUID)
			_, _ = fmt.Fprintf(w, "Datapath: %s (%s)\n", trace.Trace.DatapathName, trace.Trace.DatapathUUID)
			if trace.Trace.DstIP != "" {
				_, _ = fmt.Fprintf(w, "Destination IP: %s\n", trace.Trace.DstIP)
			}
			if trace.Trace.Protocol != "" {
				_, _ = fmt.Fprintf(w, "Protocol: %s\n", trace.Trace.Protocol)
			}
			_, _ = fmt.Fprintf(w, "\n--- Pipeline ---\n\n")
			for _, stage := range trace.Trace.Stages {
				_, _ = fmt.Fprintf(w, "[%s] Table %d: %s\n", stage.Pipeline, stage.TableID, stage.TableName)
				for _, flow := range stage.Flows {
					marker := "  "
					if flow.Selected {
						marker = "* "
					}
					_, _ = fmt.Fprintf(w, "  %spriority=%d match=%q actions=%q hint=%s\n",
						marker, flow.Priority, flow.Match, flow.Actions, flow.Hint)
				}
				_, _ = fmt.Fprintln(w)
			}
		default:
			api.WriteError(w, http.StatusBadRequest, "format must be 'json' or 'text'")
		}
	}
}

func handleListTraces(store *TraceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traces := store.List()
		if traces == nil {
			traces = []StoredTrace{}
		}
		api.WriteJSON(w, http.StatusOK, traces)
	}
}
