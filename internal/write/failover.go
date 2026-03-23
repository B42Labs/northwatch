package write

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/b42labs/northwatch/internal/ovsdb/sb"
)

// chassisMember abstracts over NB HA_Chassis and Gateway_Chassis entries,
// which both have UUID, ChassisName, and Priority.
type chassisMember struct {
	UUID        string
	Table       string // "HA_Chassis" or "Gateway_Chassis"
	ChassisName string
	Priority    int
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
		return nil, fmt.Errorf("group %q has fewer than 2 chassis entries", rg.Name)
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
		return nil, fmt.Errorf("chassis %q not found in group %q", targetChassisName, rg.Name)
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

// Evacuate generates a preview plan that fails over all HA groups where the
// given chassis is currently active to the next-highest-priority standby
// chassis. Searches both NB HA_Chassis_Groups and Gateway_Chassis (via LRPs).
// When an SB client is available, the actual active chassis is determined from
// SB Port_Binding state; otherwise it falls back to NB priority ordering.
// Returns a single plan that applies all swaps atomically.
func (e *Engine) Evacuate(ctx context.Context, chassisName string) (*Plan, error) {
	allGroups, err := e.collectAllGroups(ctx)
	if err != nil {
		return nil, err
	}

	// Build SB active chassis map when SB client is available.
	var sbActiveMap map[string]string // group key -> active chassis name
	if e.sbClient != nil {
		sbActive, sbErr := e.buildSBActiveMapUnified(ctx, allGroups)
		if sbErr != nil {
			return nil, fmt.Errorf("resolving SB active state: %w", sbErr)
		}
		sbActiveMap = sbActive
	}

	var ops []WriteOperation

	for groupKey, rg := range allGroups {
		if len(rg.Members) < 2 {
			continue
		}

		// Sort descending by priority.
		sort.Slice(rg.Members, func(i, j int) bool {
			return rg.Members[i].Priority > rg.Members[j].Priority
		})

		// Determine the actual active chassis for this group.
		activeName := rg.Members[0].ChassisName // default: highest NB priority
		if sbActiveMap != nil {
			if name, ok := sbActiveMap[groupKey]; ok {
				activeName = name
			}
		}
		if activeName != chassisName {
			// This chassis is not the SB-active chassis for this group.
			// If it has the highest NB priority (NB/SB mismatch), bump its
			// priority to trigger ovn-northd re-sync. Otherwise the chassis
			// is simply not involved in this group — skip it.
			if rg.Members[0].ChassisName == chassisName {
				leader := rg.Members[0]
				newPrio := leader.Priority + 1
				if newPrio > 32767 {
					newPrio = leader.Priority - 1
				}
				ops = append(ops, WriteOperation{
					Action: "update",
					Table:  leader.Table,
					UUID:   leader.UUID,
					Fields: map[string]any{"priority": newPrio},
					Reason: fmt.Sprintf("Evacuate '%s': bump priority in group '%s' to trigger ovn-northd re-sync",
						chassisName, rg.Name),
				})
			}
			continue
		}

		// Find the active member.
		var active chassisMember
		for _, m := range rg.Members {
			if m.ChassisName == activeName {
				active = m
				break
			}
		}

		// Find standby: highest-priority member that is not the active one.
		var standby chassisMember
		for _, m := range rg.Members {
			if m.UUID != active.UUID {
				standby = m
				break
			}
		}

		ops = append(ops, swapMemberPriorityOps(active, standby,
			fmt.Sprintf("Evacuate '%s': group '%s' — %s → %s", chassisName, rg.Name, active.ChassisName, standby.ChassisName))...)
	}

	if len(ops) == 0 {
		return nil, fmt.Errorf("chassis %q is not the active chassis in any HA group", chassisName)
	}

	return e.Preview(ctx, ops)
}

// collectAllGroups returns all HA groups from both NB HA_Chassis_Groups and
// Gateway_Chassis (via LRP references). The returned map is keyed by NB UUID
// (group UUID or LRP UUID).
func (e *Engine) collectAllGroups(ctx context.Context) (map[string]*resolvedGroup, error) {
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
			return nil, fmt.Errorf(
				"HA_Chassis_Group %q not found; chassis %q appears in %d groups",
				groupName, targetChassisName, len(candidates))
		}
	}

	return nil, fmt.Errorf("HA_Chassis_Group %q not found", groupName)
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

// buildSBActiveMapUnified queries the SB database to determine which chassis
// is actually active for each group. It matches groups by chassis name
// membership. Returns a map from group key to the active chassis name.
func (e *Engine) buildSBActiveMapUnified(ctx context.Context, groups map[string]*resolvedGroup) (map[string]string, error) {
	var sbPortBindings []sb.PortBinding
	if err := e.sbClient.List(ctx, &sbPortBindings); err != nil {
		return nil, fmt.Errorf("listing SB Port_Bindings: %w", err)
	}

	var sbChassisList []sb.Chassis
	if err := e.sbClient.List(ctx, &sbChassisList); err != nil {
		return nil, fmt.Errorf("listing SB Chassis: %w", err)
	}

	var sbGroups []sb.HAChassisGroup
	if err := e.sbClient.List(ctx, &sbGroups); err != nil {
		return nil, fmt.Errorf("listing SB HA_Chassis_Groups: %w", err)
	}

	var sbHAChassisList []sb.HAChassis
	if err := e.sbClient.List(ctx, &sbHAChassisList); err != nil {
		return nil, fmt.Errorf("listing SB HA_Chassis: %w", err)
	}

	// SB Chassis UUID -> name
	sbChassisName := make(map[string]string, len(sbChassisList))
	for _, c := range sbChassisList {
		sbChassisName[c.UUID] = c.Name
	}

	// SB HA_Chassis UUID -> entry
	sbHAChassisByUUID := make(map[string]sb.HAChassis, len(sbHAChassisList))
	for _, hc := range sbHAChassisList {
		sbHAChassisByUUID[hc.UUID] = hc
	}

	// SB group UUID -> active chassis name (from chassisredirect port bindings)
	sbGroupActive := make(map[string]string)
	for i := range sbPortBindings {
		pb := &sbPortBindings[i]
		if pb.Type != "chassisredirect" || pb.HaChassisGroup == nil || pb.Chassis == nil {
			continue
		}
		if name, ok := sbChassisName[*pb.Chassis]; ok {
			sbGroupActive[*pb.HaChassisGroup] = name
		}
	}

	// SB group -> chassis name key
	sbGroupKey := make(map[string]string, len(sbGroups))
	for _, sg := range sbGroups {
		sbGroupKey[sg.UUID] = chassisNameKey(sg.HaChassis, func(uuid string) string {
			hc, ok := sbHAChassisByUUID[uuid]
			if !ok || hc.Chassis == nil {
				return ""
			}
			return sbChassisName[*hc.Chassis]
		})
	}

	// chassis-name-key -> active chassis name
	keyToActive := make(map[string]string)
	for sgUUID, key := range sbGroupKey {
		if active, ok := sbGroupActive[sgUUID]; ok && key != "" {
			keyToActive[key] = active
		}
	}

	// Match NB groups by chassis name membership
	result := make(map[string]string, len(groups))
	for groupKey, rg := range groups {
		key := chassisNameKeyFromMembers(rg.Members)
		if key == "" {
			continue
		}
		if active, ok := keyToActive[key]; ok {
			result[groupKey] = active
		}
	}

	return result, nil
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
