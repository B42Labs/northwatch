package ovnsim

import (
	"context"
	"fmt"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
)

// ownedHAChassisByUUID loads every simulator-owned HA_Chassis indexed by UUID.
func (s *Simulator) ownedHAChassisByUUID(ctx context.Context) (map[string]nb.HAChassis, error) {
	hcs, err := listOwned(ctx, s.c, func(h *nb.HAChassis) map[string]string { return h.ExternalIDs })
	if err != nil {
		return nil, err
	}
	byUUID := make(map[string]nb.HAChassis, len(hcs))
	for _, h := range hcs {
		byUUID[h.UUID] = h
	}
	return byUUID, nil
}

// haFailover simulates a gateway failover: it swaps the highest- and
// lowest-priority members of a random owned HA_Chassis_Group, so a different
// chassis becomes the active gateway. ovn-northd then rebinds the
// chassisredirect port to the new highest-priority chassis.
func (s *Simulator) haFailover(ctx context.Context) (string, error) {
	groups, err := listOwned(ctx, s.c, func(g *nb.HAChassisGroup) map[string]string { return g.ExternalIDs })
	if err != nil {
		return "", err
	}
	byUUID, err := s.ownedHAChassisByUUID(ctx)
	if err != nil {
		return "", err
	}

	s.rng.Shuffle(len(groups), func(i, j int) { groups[i], groups[j] = groups[j], groups[i] })
	for _, g := range groups {
		hi, lo, ok := highestLowest(g.HaChassis, byUUID)
		if !ok {
			continue
		}
		t := newTxn(s.c)
		hiNew := &nb.HAChassis{UUID: hi.UUID, Priority: lo.Priority}
		t.addOps(s.c.Where(hiNew).Update(hiNew, &hiNew.Priority))
		loNew := &nb.HAChassis{UUID: lo.UUID, Priority: hi.Priority}
		t.addOps(s.c.Where(loNew).Update(loNew, &loNew.Priority))
		if err := t.commit(ctx); err != nil {
			return "", err
		}
		return fmt.Sprintf("HA failover in %s: %s now active (prio %d), %s demoted (prio %d)",
			g.Name, lo.ChassisName, hi.Priority, hi.ChassisName, lo.Priority), nil
	}
	return "skip ha.failover (no multi-member HA groups)", nil
}

// highestLowest returns the highest- and lowest-priority members of a group from
// the member UUID list, plus whether a meaningful (distinct, >=2 member) pair was
// found.
func highestLowest(memberUUIDs []string, byUUID map[string]nb.HAChassis) (hi, lo nb.HAChassis, ok bool) {
	var members []nb.HAChassis
	for _, u := range memberUUIDs {
		if h, present := byUUID[u]; present {
			members = append(members, h)
		}
	}
	if len(members) < 2 {
		return nb.HAChassis{}, nb.HAChassis{}, false
	}
	hi, lo = members[0], members[0]
	for _, m := range members {
		if m.Priority > hi.Priority {
			hi = m
		}
		if m.Priority < lo.Priority {
			lo = m
		}
	}
	if hi.UUID == lo.UUID {
		return nb.HAChassis{}, nb.HAChassis{}, false // all priorities equal
	}
	return hi, lo, true
}

// haMemberChurn adds a chassis to, or removes one from, a random owned
// HA_Chassis_Group — simulating changes to the set of gateway candidates.
func (s *Simulator) haMemberChurn(ctx context.Context) (string, error) {
	groups, err := listOwned(ctx, s.c, func(g *nb.HAChassisGroup) map[string]string { return g.ExternalIDs })
	if err != nil {
		return "", err
	}
	if len(groups) == 0 {
		return "skip ha.member (no HA groups)", nil
	}
	byUUID, err := s.ownedHAChassisByUUID(ctx)
	if err != nil {
		return "", err
	}

	g := groups[s.rng.Intn(len(groups))]

	memberNames := map[string]bool{}
	for _, u := range g.HaChassis {
		if h, ok := byUUID[u]; ok {
			memberNames[h.ChassisName] = true
		}
	}
	var available []string
	for _, ch := range s.opts.Chassis {
		if !memberNames[ch] {
			available = append(available, ch)
		}
	}

	canAdd := len(available) > 0
	canRemove := len(g.HaChassis) > 1

	if canAdd && (!canRemove || s.rng.Intn(2) == 0) {
		ch := available[s.rng.Intn(len(available))]
		t := newTxn(s.c)
		m := t.namedUUID()
		t.add(&nb.HAChassis{UUID: m, ChassisName: ch, Priority: 5, ExternalIDs: ownedIDs("ha-chassis")})
		grp := &nb.HAChassisGroup{UUID: g.UUID}
		t.addOps(s.c.Where(grp).Mutate(grp, model.Mutation{
			Field:   &grp.HaChassis,
			Mutator: ovsdb.MutateOperationInsert,
			Value:   []string{m},
		}))
		if err := t.commit(ctx); err != nil {
			return "", err
		}
		return fmt.Sprintf("add chassis %s to HA group %s", ch, g.Name), nil
	}

	if canRemove {
		u := g.HaChassis[s.rng.Intn(len(g.HaChassis))]
		name := byUUID[u].ChassisName
		grp := &nb.HAChassisGroup{UUID: g.UUID}
		ops := must(s.c.Where(grp).Mutate(grp, model.Mutation{
			Field:   &grp.HaChassis,
			Mutator: ovsdb.MutateOperationDelete,
			Value:   []string{u},
		}))
		if err := transact(ctx, s.c, ops); err != nil {
			return "", err
		}
		return fmt.Sprintf("remove chassis %s from HA group %s", name, g.Name), nil
	}

	return "skip ha.member (nothing to change)", nil
}
