// Package ovscorrelate joins live per-chassis OVS interface state to OVN intent
// held in the Southbound database. Given an OVS Interface's
// external_ids:iface-id, it resolves the SB Port_Binding whose logical_port
// matches, surfacing the bound logical port, its up state, datapath and bound
// chassis next to the live interface — and flags drift where SB reports the
// port up but the live interface is down or erroring. It reads only the
// existing Southbound cache and degrades gracefully when the iface-id or the SB
// mapping is absent.
package ovscorrelate

import (
	"context"
	"errors"
	"fmt"

	"github.com/ovn-kubernetes/libovsdb/client"

	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// Correlator resolves OVS interface state against the Southbound cache. It holds
// only the SB client — no NB dependency and no new OVSDB connections.
type Correlator struct {
	SB client.Client
}

// LiveInterface is the subset of a live OVS Interface the correlation needs: the
// chassis system-id it was read from, its external_ids:iface-id, and the link
// state and error fields that drive drift detection. Empty strings stand in for
// absent OVSDB values.
type LiveInterface struct {
	SystemID  string
	IfaceID   string
	LinkState string
	Error     string
}

// Binding is the SB Port_Binding realizing an OVS interface, with its chassis
// and datapath resolved to human labels.
type Binding struct {
	UUID         string `json:"uuid"`
	LogicalPort  string `json:"logical_port"`
	Type         string `json:"type,omitempty"`
	Up           *bool  `json:"up,omitempty"`
	Chassis      string `json:"chassis,omitempty"`
	BoundHere    bool   `json:"bound_here"`
	Datapath     string `json:"datapath,omitempty"`
	DatapathUUID string `json:"datapath_uuid,omitempty"`
}

// Correlation is the result of joining one OVS interface to OVN intent. Bound is
// false — with Binding nil — when the interface carries no iface-id or when no
// SB Port_Binding matches it, so callers can render the "not OVN-managed" and
// "no Port_Binding" cases without a nil check on Binding alone.
type Correlation struct {
	IfaceID string   `json:"iface_id,omitempty"`
	Bound   bool     `json:"bound"`
	Binding *Binding `json:"binding,omitempty"`
	Drift   []string `json:"drift,omitempty"`
}

// ForInterface correlates a live OVS interface with the Southbound database. It
// returns an unbound Correlation (Bound false, Binding nil) when the interface
// has no iface-id or when no Port_Binding realizes it, and only returns an error
// when the SB cache query itself fails. Chassis and datapath resolution is
// best-effort: a failed sub-lookup leaves the field empty rather than failing
// the whole correlation.
func (c *Correlator) ForInterface(ctx context.Context, live LiveInterface) (Correlation, error) {
	if live.IfaceID == "" {
		return Correlation{}, nil
	}

	// logical_port is a schema-unique index on Port_Binding, so Get resolves it
	// through the cache index in O(1) rather than scanning the whole table.
	pb := sb.PortBinding{LogicalPort: live.IfaceID}
	if err := c.SB.Get(ctx, &pb); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return Correlation{IfaceID: live.IfaceID, Bound: false}, nil
		}
		return Correlation{}, fmt.Errorf("getting port binding for iface-id %s: %w", live.IfaceID, err)
	}

	binding := &Binding{
		UUID:        pb.UUID,
		LogicalPort: pb.LogicalPort,
		Type:        pb.Type,
		Up:          pb.Up,
	}
	c.resolveChassis(ctx, &pb, live.SystemID, binding)
	c.resolveDatapath(ctx, &pb, binding)

	return Correlation{
		IfaceID: live.IfaceID,
		Bound:   true,
		Binding: binding,
		Drift:   drift(&pb, live),
	}, nil
}

// resolveChassis fills the binding's bound chassis name and whether it is bound
// on the same chassis the interface was read from. Best-effort: an unresolvable
// or absent chassis leaves the fields at their zero value.
func (c *Correlator) resolveChassis(ctx context.Context, pb *sb.PortBinding, systemID string, binding *Binding) {
	if pb.Chassis == nil || *pb.Chassis == "" {
		return
	}
	ch := &sb.Chassis{UUID: *pb.Chassis}
	if err := c.SB.Get(ctx, ch); err != nil {
		return
	}
	binding.Chassis = ch.Name
	binding.BoundHere = ch.Name == systemID
}

// resolveDatapath fills the binding's datapath UUID and label. Best-effort: the
// UUID reference is always recorded, but the human label is left empty when the
// Datapath_Binding cannot be resolved.
func (c *Correlator) resolveDatapath(ctx context.Context, pb *sb.PortBinding, binding *Binding) {
	if pb.Datapath == "" {
		return
	}
	binding.DatapathUUID = pb.Datapath
	dp := &sb.DatapathBinding{UUID: pb.Datapath}
	if err := c.SB.Get(ctx, dp); err != nil {
		return
	}
	binding.Datapath = datapathLabel(dp)
}

// datapathLabel returns the friendliest name for a datapath: its external_ids
// name, else its name2, else a tunnel-key fallback so the label is never empty.
func datapathLabel(dp *sb.DatapathBinding) string {
	if name := dp.ExternalIDs["name"]; name != "" {
		return name
	}
	if name := dp.ExternalIDs["name2"]; name != "" {
		return name
	}
	return fmt.Sprintf("tunnel_key %d", dp.TunnelKey)
}

// drift flags control-plane/data-plane divergence: the SB reports the port up
// while the live OVS interface is down or erroring. It returns nil when the
// Port_Binding is not up (nothing to diverge from) or the interface is healthy.
func drift(pb *sb.PortBinding, live LiveInterface) []string {
	if pb.Up == nil || !*pb.Up {
		return nil
	}
	var notes []string
	if live.LinkState == "down" {
		notes = append(notes, "SB reports the port up but the OVS interface link_state is down")
	}
	if live.Error != "" {
		notes = append(notes, fmt.Sprintf("SB reports the port up but the OVS interface reports error: %s", live.Error))
	}
	return notes
}
