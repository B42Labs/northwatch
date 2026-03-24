package write

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// drainPriorityKey is the external_ids key used to store the original priority
// before draining a chassis. This allows Restore to return the chassis to its
// original priority instead of a hardcoded value.
const drainPriorityKey = "northwatch:pre-drain-priority"

// chassisMember abstracts over NB HA_Chassis and Gateway_Chassis entries,
// which both have UUID, ChassisName, and Priority.
type chassisMember struct {
	UUID        string
	Table       string // "HA_Chassis" or "Gateway_Chassis"
	ChassisName string
	Priority    int
	ExternalIDs map[string]string
}

// resolvedGroup holds a group name and its chassis members.
type resolvedGroup struct {
	Name    string
	Members []chassisMember
}

// Failover generates a preview plan that swaps the priorities of the current
// active chassis (highest priority) and the target chassis within the given
// HA group. Searches both NB HA_Chassis_Groups and Gateway_Chassis (via LRPs).
// The returned plan can be applied via Engine.Apply.
func (e *Engine) Failover(ctx context.Context, groupName, targetChassisName string) (*Plan, error) {
	rg, err := e.resolveGroupUnified(ctx, groupName, targetChassisName)
	if err != nil {
		return nil, err
	}

	members := rg.Members
	if len(members) < 2 {
		return nil, &InputError{Message: fmt.Sprintf("group %q has fewer than 2 chassis entries", rg.Name)}
	}

	// Sort descending by priority — first entry is the current NB leader.
	sort.Slice(members, func(i, j int) bool {
		return members[i].Priority > members[j].Priority
	})
	active := members[0]

	// Find target.
	var target *chassisMember
	for i := range members {
		if members[i].ChassisName == targetChassisName {
			target = &members[i]
			break
		}
	}
	if target == nil {
		return nil, &InputError{Message: fmt.Sprintf("chassis %q not found in group %q", targetChassisName, rg.Name)}
	}

	if target.UUID == active.UUID {
		// Target already has the highest NB priority but isn't the SB-active
		// chassis (otherwise the UI wouldn't offer this action). This is an
		// NB/SB mismatch — ovn-northd hasn't synced the NB priorities to SB.
		// Bump the priority to trigger ovn-northd to re-evaluate and update SB.
		// Note: if another member happens to have priority == newPrio, this
		// creates a tie, but ovn-northd will still re-evaluate on any change.
		newPrio := target.Priority + 1
		if newPrio > 32767 {
			newPrio = target.Priority - 1
		}
		ops := []WriteOperation{{
			Action: "update",
			Table:  target.Table,
			UUID:   target.UUID,
			Fields: map[string]any{"priority": newPrio},
			Reason: fmt.Sprintf("HA Failover: bump priority for '%s' in group '%s' to trigger ovn-northd re-sync",
				target.ChassisName, rg.Name),
		}}
		return e.Preview(ctx, ops)
	}

	ops := swapMemberPriorityOps(active, *target,
		fmt.Sprintf("HA Failover: group '%s' — %s → %s", rg.Name, active.ChassisName, target.ChassisName))

	return e.Preview(ctx, ops)
}

// Evacuate generates a preview plan that drains a chassis from all HA groups
// by setting its priority to 0. This follows the drain approach used by
// ovn-route-agent: rather than swapping priorities with a standby, the chassis
// priority is set to 0, letting OVN's native HA mechanism promote the
// next-highest-priority chassis automatically.
// Searches both NB HA_Chassis_Groups and Gateway_Chassis (via LRPs).
// The drained chassis can later be restored to standby via Engine.Restore.
func (e *Engine) Evacuate(ctx context.Context, chassisName string) (*Plan, error) {
	allGroups, err := e.collectAllGroups(ctx)
	if err != nil {
		return nil, err
	}

	var ops []WriteOperation

	for _, rg := range allGroups {
		for _, m := range rg.Members {
			if m.ChassisName == chassisName && m.Priority != 0 {
				// Merge the drain marker into existing external_ids.
				extIDs := mergeExternalIDs(m.ExternalIDs, drainPriorityKey, strconv.Itoa(m.Priority))
				ops = append(ops, WriteOperation{
					Action: "update",
					Table:  m.Table,
					UUID:   m.UUID,
					Fields: map[string]any{
						"priority":     0,
						"external_ids": extIDs,
					},
					Reason: fmt.Sprintf("Drain '%s': set priority to 0 in group '%s' (was %d)",
						chassisName, rg.Name, m.Priority),
				})
			}
		}
	}

	if len(ops) == 0 {
		return nil, &InputError{Message: fmt.Sprintf("chassis %q has no entries to drain (not found or already drained)", chassisName)}
	}

	sortOpsByUUID(ops)
	return e.Preview(ctx, ops)
}

// Restore generates a preview plan that restores a previously drained chassis
// by setting its priority back to 1 (standby). This is the counterpart to
// Evacuate: after a chassis has been drained (priority 0), Restore brings it
// back as a standby member without disrupting the current active chassis.
// Follows the restore approach from ovn-route-agent's RestoreDrainedGateways.
func (e *Engine) Restore(ctx context.Context, chassisName string) (*Plan, error) {
	allGroups, err := e.collectAllGroups(ctx)
	if err != nil {
		return nil, err
	}

	var ops []WriteOperation

	for _, rg := range allGroups {
		for _, m := range rg.Members {
			if m.ChassisName == chassisName && m.Priority == 0 {
				// Restore to original priority from external_ids, defaulting to 1.
				restorePrio := 1
				if orig, ok := m.ExternalIDs[drainPriorityKey]; ok {
					if parsed, err := strconv.Atoi(orig); err == nil && parsed > 0 {
						restorePrio = parsed
					}
				}
				// Remove the drain marker from external_ids.
				extIDs := removeExternalID(m.ExternalIDs, drainPriorityKey)
				ops = append(ops, WriteOperation{
					Action: "update",
					Table:  m.Table,
					UUID:   m.UUID,
					Fields: map[string]any{
						"priority":     restorePrio,
						"external_ids": extIDs,
					},
					Reason: fmt.Sprintf("Restore '%s': set priority to %d in group '%s'",
						chassisName, restorePrio, rg.Name),
				})
			}
		}
	}

	if len(ops) == 0 {
		return nil, &InputError{Message: fmt.Sprintf("chassis %q has no drained entries to restore", chassisName)}
	}

	sortOpsByUUID(ops)
	return e.Preview(ctx, ops)
}

// collectAllGroups returns all HA groups from both NB HA_Chassis_Groups and
// Gateway_Chassis (via LRP references). The returned map is keyed by NB UUID
// (group UUID or LRP UUID).
func (e *Engine) collectAllGroups(ctx context.Context) (map[string]*resolvedGroup, error) {
	if e.nbClient == nil {
		return nil, fmt.Errorf("NB client not available")
	}

	result := make(map[string]*resolvedGroup)

	// --- HA_Chassis_Groups ---
	var haGroups []nb.HAChassisGroup
	if err := e.nbClient.List(ctx, &haGroups); err != nil {
		return nil, fmt.Errorf("listing HA_Chassis_Groups: %w", err)
	}
	var haChassis []nb.HAChassis
	if err := e.nbClient.List(ctx, &haChassis); err != nil {
		return nil, fmt.Errorf("listing HA_Chassis: %w", err)
	}
	hcByUUID := make(map[string]nb.HAChassis, len(haChassis))
	for _, hc := range haChassis {
		hcByUUID[hc.UUID] = hc
	}
	for _, g := range haGroups {
		var members []chassisMember
		for _, uuid := range g.HaChassis {
			if hc, ok := hcByUUID[uuid]; ok {
				members = append(members, chassisMember{
					UUID: hc.UUID, Table: "HA_Chassis",
					ChassisName: hc.ChassisName, Priority: hc.Priority,
					ExternalIDs: hc.ExternalIDs,
				})
			}
		}
		result[g.UUID] = &resolvedGroup{Name: g.Name, Members: members}
	}

	// --- Gateway_Chassis (via LRPs) ---
	var lrps []nb.LogicalRouterPort
	if err := e.nbClient.List(ctx, &lrps); err != nil {
		return nil, fmt.Errorf("listing Logical_Router_Ports: %w", err)
	}
	var gwChassis []nb.GatewayChassis
	if err := e.nbClient.List(ctx, &gwChassis); err != nil {
		return nil, fmt.Errorf("listing Gateway_Chassis: %w", err)
	}
	gwByUUID := make(map[string]nb.GatewayChassis, len(gwChassis))
	for _, gw := range gwChassis {
		gwByUUID[gw.UUID] = gw
	}
	for _, lrp := range lrps {
		if len(lrp.GatewayChassis) == 0 {
			continue
		}
		var members []chassisMember
		for _, uuid := range lrp.GatewayChassis {
			if gw, ok := gwByUUID[uuid]; ok {
				members = append(members, chassisMember{
					UUID: gw.UUID, Table: "Gateway_Chassis",
					ChassisName: gw.ChassisName, Priority: gw.Priority,
					ExternalIDs: gw.ExternalIDs,
				})
			}
		}
		if len(members) > 0 {
			result[lrp.UUID] = &resolvedGroup{Name: lrp.Name, Members: members}
		}
	}

	return result, nil
}

// resolveGroupUnified finds an HA group by name, searching both
// HA_Chassis_Groups and Gateway_Chassis (via LRPs). If exact name match fails,
// it falls back to finding the group that contains the target chassis.
func (e *Engine) resolveGroupUnified(ctx context.Context, groupName, targetChassisName string) (*resolvedGroup, error) {
	allGroups, err := e.collectAllGroups(ctx)
	if err != nil {
		return nil, err
	}

	// Try exact name match first.
	for _, rg := range allGroups {
		if rg.Name == groupName {
			return rg, nil
		}
	}

	// Fallback: find the group containing the target chassis.
	if targetChassisName != "" {
		var candidates []*resolvedGroup
		for _, rg := range allGroups {
			for _, m := range rg.Members {
				if m.ChassisName == targetChassisName {
					candidates = append(candidates, rg)
					break
				}
			}
		}
		if len(candidates) == 1 {
			return candidates[0], nil
		}
		if len(candidates) > 1 {
			// Use SB to disambiguate if possible.
			if e.sbClient != nil {
				if found := e.disambiguateViaSBUnified(ctx, groupName, candidates); found != nil {
					return found, nil
				}
			}
			return nil, &InputError{Message: fmt.Sprintf(
				"HA_Chassis_Group %q not found; chassis %q appears in %d groups",
				groupName, targetChassisName, len(candidates))}
		}
	}

	return nil, &InputError{Message: fmt.Sprintf("HA_Chassis_Group %q not found", groupName)}
}

// disambiguateViaSBUnified finds the candidate whose chassis membership
// matches the SB group with the given name.
func (e *Engine) disambiguateViaSBUnified(ctx context.Context, sbGroupName string, candidates []*resolvedGroup) *resolvedGroup {
	var sbGroups []sb.HAChassisGroup
	if err := e.sbClient.List(ctx, &sbGroups); err != nil {
		return nil
	}

	var sbGroup *sb.HAChassisGroup
	for i := range sbGroups {
		if sbGroups[i].Name == sbGroupName {
			sbGroup = &sbGroups[i]
			break
		}
	}
	if sbGroup == nil {
		return nil
	}

	var sbHAChassisList []sb.HAChassis
	if err := e.sbClient.List(ctx, &sbHAChassisList); err != nil {
		return nil
	}
	sbHAChassisByUUID := make(map[string]sb.HAChassis, len(sbHAChassisList))
	for _, hc := range sbHAChassisList {
		sbHAChassisByUUID[hc.UUID] = hc
	}

	var sbChassisList []sb.Chassis
	if err := e.sbClient.List(ctx, &sbChassisList); err != nil {
		return nil
	}
	sbChassisNameMap := make(map[string]string, len(sbChassisList))
	for _, c := range sbChassisList {
		sbChassisNameMap[c.UUID] = c.Name
	}

	sbKey := chassisNameKey(sbGroup.HaChassis, func(uuid string) string {
		hc, ok := sbHAChassisByUUID[uuid]
		if !ok || hc.Chassis == nil {
			return ""
		}
		return sbChassisNameMap[*hc.Chassis]
	})
	if sbKey == "" {
		return nil
	}

	for _, c := range candidates {
		nbKey := chassisNameKeyFromMembers(c.Members)
		if nbKey == sbKey {
			return c
		}
	}

	return nil
}

// chassisNameKey builds a stable key from a list of UUIDs by resolving each
// to a chassis name, sorting, and joining with null bytes.
func chassisNameKey(uuids []string, resolve func(string) string) string {
	var names []string
	for _, uuid := range uuids {
		name := resolve(uuid)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return strings.Join(names, "\x00")
}

// chassisNameKeyFromMembers builds a stable key from chassisMember entries.
func chassisNameKeyFromMembers(members []chassisMember) string {
	var names []string
	for _, m := range members {
		if m.ChassisName != "" {
			names = append(names, m.ChassisName)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return strings.Join(names, "\x00")
}

// swapMemberPriorityOps builds WriteOperations that swap the priorities of two
// chassis members. The Table field on each member determines which NB table
// is targeted (HA_Chassis or Gateway_Chassis).
func swapMemberPriorityOps(a, b chassisMember, reason string) []WriteOperation {
	return []WriteOperation{
		{
			Action: "update",
			Table:  a.Table,
			UUID:   a.UUID,
			Fields: map[string]any{"priority": b.Priority},
			Reason: reason,
		},
		{
			Action: "update",
			Table:  b.Table,
			UUID:   b.UUID,
			Fields: map[string]any{"priority": a.Priority},
			Reason: reason,
		},
	}
}

// mergeExternalIDs returns a copy of the existing external_ids map with the
// given key/value pair added. If existing is nil, a new map is created.
func mergeExternalIDs(existing map[string]string, key, value string) map[string]string {
	merged := make(map[string]string, len(existing)+1)
	for k, v := range existing {
		merged[k] = v
	}
	merged[key] = value
	return merged
}

// removeExternalID returns a copy of the existing external_ids map with the
// given key removed. Returns an empty map (not nil) if the result would be empty,
// since OVSDB expects an empty map rather than nil for map columns.
func removeExternalID(existing map[string]string, key string) map[string]string {
	result := make(map[string]string, len(existing))
	for k, v := range existing {
		if k != key {
			result[k] = v
		}
	}
	return result
}

// sortOpsByUUID sorts WriteOperations by UUID for deterministic plan output.
func sortOpsByUUID(ops []WriteOperation) {
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].UUID < ops[j].UUID
	})
}
