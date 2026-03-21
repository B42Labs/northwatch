package debug

import (
	"context"
	"fmt"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
	"github.com/ovn-kubernetes/libovsdb/client"
)

// StaleEntry represents a single stale entry finding.
type StaleEntry struct {
	Type       string         `json:"type"`
	Severity   string         `json:"severity"`
	UUID       string         `json:"uuid"`
	Table      string         `json:"table"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
	AgeSeconds int64          `json:"age_seconds,omitempty"`
}

// StaleEntriesResult aggregates all stale entries.
type StaleEntriesResult struct {
	Total         int          `json:"total"`
	StaleMAC      int          `json:"stale_mac_bindings"`
	OrphanedFDB   int          `json:"orphaned_fdb"`
	OrphanedPorts int          `json:"orphaned_port_bindings"`
	Entries       []StaleEntry `json:"entries"`
}

// StaleDetector finds stale entries across NB and SB databases.
type StaleDetector struct {
	NB     client.Client
	SB     client.Client
	MaxAge time.Duration
}

// DetectAll runs all stale entry checks.
func (d *StaleDetector) DetectAll(ctx context.Context) (*StaleEntriesResult, error) {
	result := &StaleEntriesResult{Entries: []StaleEntry{}}

	maxAge := d.MaxAge
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}

	staleMacs, err := d.detectStaleMACBindings(ctx, maxAge)
	if err != nil {
		return nil, fmt.Errorf("checking MAC bindings: %w", err)
	}
	result.Entries = append(result.Entries, staleMacs...)
	result.StaleMAC = len(staleMacs)

	orphanedFDB, err := d.detectOrphanedFDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking FDB entries: %w", err)
	}
	result.Entries = append(result.Entries, orphanedFDB...)
	result.OrphanedFDB = len(orphanedFDB)

	orphanedPorts, err := d.detectOrphanedPortBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking port bindings: %w", err)
	}
	result.Entries = append(result.Entries, orphanedPorts...)
	result.OrphanedPorts = len(orphanedPorts)

	result.Total = len(result.Entries)
	return result, nil
}

func (d *StaleDetector) detectStaleMACBindings(ctx context.Context, maxAge time.Duration) ([]StaleEntry, error) {
	var macs []sb.MACBinding
	if err := d.SB.List(ctx, &macs); err != nil {
		return nil, err
	}

	now := time.Now()
	cutoff := now.Add(-maxAge)
	var entries []StaleEntry

	for _, m := range macs {
		if m.Timestamp == 0 {
			continue
		}
		bindingTime := time.UnixMilli(int64(m.Timestamp))
		if bindingTime.Before(cutoff) {
			age := now.Sub(bindingTime)
			entries = append(entries, StaleEntry{
				Type:     "mac_binding",
				Severity: "warning",
				UUID:     m.UUID,
				Table:    "MAC_Binding",
				Message:  fmt.Sprintf("MAC binding %s -> %s is %.0f hours old", m.IP, m.MAC, age.Hours()),
				Details: map[string]any{
					"ip":           m.IP,
					"mac":          m.MAC,
					"logical_port": m.LogicalPort,
					"datapath":     m.Datapath,
					"timestamp":    bindingTime.UTC().Format(time.RFC3339),
				},
				AgeSeconds: int64(age.Seconds()),
			})
		}
	}
	return entries, nil
}

func (d *StaleDetector) detectOrphanedFDB(ctx context.Context) ([]StaleEntry, error) {
	var fdbs []sb.FDB
	if err := d.SB.List(ctx, &fdbs); err != nil {
		return nil, err
	}

	var pbs []sb.PortBinding
	if err := d.SB.List(ctx, &pbs); err != nil {
		return nil, err
	}
	validPortKeys := make(map[int]bool, len(pbs))
	for _, pb := range pbs {
		validPortKeys[pb.TunnelKey] = true
	}

	var entries []StaleEntry
	for _, f := range fdbs {
		if !validPortKeys[f.PortKey] {
			entries = append(entries, StaleEntry{
				Type:     "fdb",
				Severity: "warning",
				UUID:     f.UUID,
				Table:    "FDB",
				Message:  fmt.Sprintf("FDB entry for MAC %s references port key %d which has no port binding", f.MAC, f.PortKey),
				Details: map[string]any{
					"mac":      f.MAC,
					"port_key": f.PortKey,
					"dp_key":   f.DpKey,
				},
			})
		}
	}
	return entries, nil
}

func (d *StaleDetector) detectOrphanedPortBindings(ctx context.Context) ([]StaleEntry, error) {
	var pbs []sb.PortBinding
	if err := d.SB.List(ctx, &pbs); err != nil {
		return nil, err
	}

	var lsps []nb.LogicalSwitchPort
	if err := d.NB.List(ctx, &lsps); err != nil {
		return nil, err
	}
	knownPorts := make(map[string]bool, len(lsps))
	for _, lsp := range lsps {
		knownPorts[lsp.Name] = true
	}

	var lrps []nb.LogicalRouterPort
	if err := d.NB.List(ctx, &lrps); err != nil {
		return nil, err
	}
	for _, lrp := range lrps {
		knownPorts[lrp.Name] = true
		knownPorts["cr-"+lrp.Name] = true
	}

	var entries []StaleEntry
	for _, pb := range pbs {
		if pb.Type == "localnet" || pb.Type == "l2gateway" || pb.Type == "l3gateway" || pb.Type == "localport" || pb.Type == "vtep" {
			continue
		}
		if !knownPorts[pb.LogicalPort] {
			entries = append(entries, StaleEntry{
				Type:     "port_binding",
				Severity: "error",
				UUID:     pb.UUID,
				Table:    "Port_Binding",
				Message:  fmt.Sprintf("Port binding %q has no corresponding NB entity", pb.LogicalPort),
				Details: map[string]any{
					"logical_port": pb.LogicalPort,
					"type":         pb.Type,
					"datapath":     pb.Datapath,
				},
			})
		}
	}
	return entries, nil
}
