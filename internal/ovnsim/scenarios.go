package ovnsim

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
)

// boundChassisKey is the external_ids key ovnsim uses to remember which chassis
// a VIF port is currently bound to (so migrate/unbind know where it lives).
const boundChassisKey = "nw-bound-chassis"

// SimConfig configures the continuous simulator.
type SimConfig struct {
	Options  Options          // topology shape used when creating switches/routers
	Target   int              // homeostasis anchor: desired number of owned switches
	RandSeed int64            // PRNG seed for reproducible action sequences
	Binder   *Binder          // optional; enables real port binding onto chassis
	Logf     func(string, ...any)
}

// Simulator drives continuous, bounded change against an OVN NB database. Every
// object it touches is one it owns (see SimTag); it never mutates foreign rows.
type Simulator struct {
	c      client.Client
	rng    *rand.Rand
	opts   Options
	target int
	binder *Binder
	logf   func(string, ...any)
	seq    int
}

// NewSimulator builds a simulator. Target defaults to the configured switch
// count when not set.
func NewSimulator(c client.Client, cfg SimConfig) *Simulator {
	opts := cfg.Options.withDefaults()
	target := cfg.Target
	if target <= 0 {
		target = opts.Switches
	}
	logf := cfg.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Simulator{
		c:      c,
		rng:    newRand(cfg.RandSeed),
		opts:   opts,
		target: target,
		binder: cfg.Binder,
		logf:   logf,
	}
}

// newRand builds the simulator's PRNG. A seedable, non-cryptographic source is
// deliberate: action sequences must be reproducible via --rand-seed, and the
// simulator is not security-sensitive.
func newRand(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed)) //nolint:gosec // intentional deterministic PRNG
}

func (s *Simulator) nextSeq() int { s.seq++; return s.seq }

// Run performs one action every interval until the context is cancelled.
func (s *Simulator) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			desc, err := s.Step(ctx)
			if err != nil {
				s.logf("step error: %v", err)
				continue
			}
			s.logf("%s", desc)
		}
	}
}

// Step performs a single weighted-random action and returns a short description.
func (s *Simulator) Step(ctx context.Context) (string, error) {
	switches, err := listOwned(ctx, s.c, func(l *nb.LogicalSwitch) map[string]string { return l.ExternalIDs })
	if err != nil {
		return "", err
	}
	action := chooseAction(s.rng, len(switches), s.target, s.binder != nil)
	return s.run(ctx, action)
}

// chooseAction picks the next action key using weights that bias switch
// creation vs. deletion toward the target count (homeostasis), keeping the
// owned object set lively but bounded. Deterministic for a given rng.
func chooseAction(rng *rand.Rand, switches, target int, hasBinder bool) string {
	type wa struct {
		key string
		w   int
	}
	actions := []wa{
		{"port.add", 4}, {"port.remove", 3}, {"port.toggle", 3},
		{"router.create", 2}, {"router.delete", 1},
		{"nat.add", 2}, {"nat.remove", 2},
		{"acl.toggle", 2},
		{"lb.vip.add", 2}, {"lb.vip.remove", 1},
		{"ha.failover", 3}, {"ha.member", 2},
	}
	switch {
	case switches < target:
		actions = append(actions, wa{"switch.create", 6}, wa{"switch.delete", 1})
	case switches > target:
		actions = append(actions, wa{"switch.create", 1}, wa{"switch.delete", 6})
	default:
		actions = append(actions, wa{"switch.create", 3}, wa{"switch.delete", 3})
	}
	if hasBinder {
		// New ports are bound on creation (see addPort/createSwitch), so the
		// standalone bind action only mops up stragglers; migrate moves a binding
		// to another chassis. There is deliberately no random "unbind" action — it
		// would leave VIFs unbound and flood the dashboard with health alerts. Use
		// `ovnsim unbind` / `make lab-unbind` to demo unbinding explicitly.
		actions = append(actions, wa{"bind", 2}, wa{"migrate", 4})
	}

	total := 0
	for _, a := range actions {
		total += a.w
	}
	n := rng.Intn(total)
	for _, a := range actions {
		if n < a.w {
			return a.key
		}
		n -= a.w
	}
	return actions[len(actions)-1].key
}

func (s *Simulator) run(ctx context.Context, action string) (string, error) {
	switch action {
	case "switch.create":
		return s.createSwitch(ctx)
	case "switch.delete":
		return s.deleteSwitch(ctx)
	case "router.create":
		return s.createRouter(ctx)
	case "router.delete":
		return s.deleteRouter(ctx)
	case "port.add":
		return s.addPort(ctx)
	case "port.remove":
		return s.removePort(ctx)
	case "port.toggle":
		return s.togglePort(ctx)
	case "nat.add":
		return s.addNAT(ctx)
	case "nat.remove":
		return s.removeNAT(ctx)
	case "acl.toggle":
		return s.toggleACL(ctx)
	case "lb.vip.add":
		return s.addLBVip(ctx)
	case "lb.vip.remove":
		return s.removeLBVip(ctx)
	case "ha.failover":
		return s.haFailover(ctx)
	case "ha.member":
		return s.haMemberChurn(ctx)
	case "bind":
		return s.bindPort(ctx)
	case "migrate":
		return s.migratePort(ctx)
	default:
		return "noop", nil
	}
}

// --- switches & routers ---------------------------------------------------

func (s *Simulator) createSwitch(ctx context.Context) (string, error) {
	sws, err := listOwned(ctx, s.c, func(l *nb.LogicalSwitch) map[string]string { return l.ExternalIDs })
	if err != nil {
		return "", err
	}
	idx := freeIndex(usedIndices(switchNames(sws), NamePrefix+"ls-"))
	if err := seedSwitch(ctx, s.c, s.opts, idx, newSeedResult()); err != nil {
		return "", err
	}

	// Bind the new switch's VIFs immediately when binding is enabled, so a fresh
	// switch never shows up as a wall of "unbound VIF" alerts.
	bound := 0
	if s.binder != nil && len(s.binder.Chassis) > 0 {
		for p := 1; p <= s.opts.PortsPerSwitch; p++ {
			name := vifName(idx, p)
			chassis := s.binder.Chassis[s.rng.Intn(len(s.binder.Chassis))]
			if err := s.binder.Bind(ctx, chassis, name); err != nil {
				return "", err
			}
			if err := recordBoundChassisByName(ctx, s.c, name, chassis); err != nil {
				return "", err
			}
			bound++
		}
	}
	if bound > 0 {
		return fmt.Sprintf("create switch %s (%d ports bound)", switchName(idx), bound), nil
	}
	return "create switch " + switchName(idx), nil
}

func (s *Simulator) deleteSwitch(ctx context.Context) (string, error) {
	sws, err := listOwned(ctx, s.c, func(l *nb.LogicalSwitch) map[string]string { return l.ExternalIDs })
	if err != nil {
		return "", err
	}
	if len(sws) <= 1 {
		return "skip switch.delete (keeping at least one)", nil
	}
	sw := sws[s.rng.Intn(len(sws))]
	if err := transact(ctx, s.c, must(s.c.Where(&nb.LogicalSwitch{UUID: sw.UUID}).Delete())); err != nil {
		return "", err
	}
	return "delete switch " + sw.Name, nil
}

func (s *Simulator) createRouter(ctx context.Context) (string, error) {
	rs, err := listOwned(ctx, s.c, func(r *nb.LogicalRouter) map[string]string { return r.ExternalIDs })
	if err != nil {
		return "", err
	}
	idx := freeIndex(usedIndices(routerNamesOf(rs), NamePrefix+"lr-"))
	o := octet(idx)
	t := newTxn(s.c)
	lrpUUID := t.namedUUID()
	t.add(&nb.LogicalRouterPort{
		UUID:        lrpUUID,
		Name:        fmt.Sprintf("%slr-%03d-p", NamePrefix, idx),
		MAC:         mac(0x400000 + idx),
		Networks:    []string{fmt.Sprintf("10.%d.255.1/24", o)},
		ExternalIDs: ownedIDs("router-port"),
	})
	natUUID := t.namedUUID()
	t.add(&nb.NAT{
		UUID:        natUUID,
		Type:        nb.NATTypeSNAT,
		ExternalIP:  "192.0.2.200",
		LogicalIP:   fmt.Sprintf("10.%d.255.0/24", o),
		ExternalIDs: ownedIDs("nat"),
	})
	t.add(&nb.LogicalRouter{
		UUID:        t.namedUUID(),
		Name:        routerName(idx),
		Ports:       []string{lrpUUID},
		Nat:         []string{natUUID},
		ExternalIDs: ownedIDs("router"),
	})
	if err := t.commit(ctx); err != nil {
		return "", err
	}
	return "create router " + routerName(idx), nil
}

func (s *Simulator) deleteRouter(ctx context.Context) (string, error) {
	rs, err := listOwned(ctx, s.c, func(r *nb.LogicalRouter) map[string]string { return r.ExternalIDs })
	if err != nil {
		return "", err
	}
	if len(rs) == 0 {
		return "skip router.delete (none)", nil
	}
	r := rs[s.rng.Intn(len(rs))]
	if err := transact(ctx, s.c, must(s.c.Where(&nb.LogicalRouter{UUID: r.UUID}).Delete())); err != nil {
		return "", err
	}
	return "delete router " + r.Name, nil
}

// --- ports ----------------------------------------------------------------

func (s *Simulator) addPort(ctx context.Context) (string, error) {
	sws, err := listOwned(ctx, s.c, func(l *nb.LogicalSwitch) map[string]string { return l.ExternalIDs })
	if err != nil {
		return "", err
	}
	if len(sws) == 0 {
		return "skip port.add (no switches)", nil
	}
	sw := sws[s.rng.Intn(len(sws))]
	name := fmt.Sprintf("%s-p%d", sw.Name, s.nextSeq())

	// When binding is enabled, mark the new port bound up front and bind it after
	// commit, so it never lingers as an unbound VIF.
	ids := ownedIDs("vif")
	chassis := s.pickBindChassis()
	if chassis != "" {
		ids[boundChassisKey] = chassis
	}

	t := newTxn(s.c)
	lspUUID := t.namedUUID()
	t.add(&nb.LogicalSwitchPort{
		UUID:        lspUUID,
		Name:        name,
		Addresses:   []string{"dynamic"},
		Enabled:     ptr(true),
		ExternalIDs: ids,
	})
	ls := &nb.LogicalSwitch{UUID: sw.UUID}
	t.addOps(s.c.Where(ls).Mutate(ls, model.Mutation{
		Field:   &ls.Ports,
		Mutator: ovsdb.MutateOperationInsert,
		Value:   []string{lspUUID},
	}))
	if err := t.commit(ctx); err != nil {
		return "", err
	}

	if chassis != "" {
		if err := s.binder.Bind(ctx, chassis, name); err != nil {
			return "", err
		}
		return fmt.Sprintf("add port %s to %s (bound to %s)", name, sw.Name, chassis), nil
	}
	return fmt.Sprintf("add port %s to %s", name, sw.Name), nil
}

// pickBindChassis returns a random chassis to bind a new port onto, or "" when
// port binding is disabled.
func (s *Simulator) pickBindChassis() string {
	if s.binder == nil || len(s.binder.Chassis) == 0 {
		return ""
	}
	return s.binder.Chassis[s.rng.Intn(len(s.binder.Chassis))]
}

func (s *Simulator) removePort(ctx context.Context) (string, error) {
	vifs, err := listOwned(ctx, s.c, func(p *nb.LogicalSwitchPort) map[string]string { return p.ExternalIDs })
	if err != nil {
		return "", err
	}
	vifByUUID := map[string]nb.LogicalSwitchPort{}
	for _, p := range vifs {
		if p.ExternalIDs["nw-kind"] == "vif" {
			vifByUUID[p.UUID] = p
		}
	}
	sws, err := listOwned(ctx, s.c, func(l *nb.LogicalSwitch) map[string]string { return l.ExternalIDs })
	if err != nil {
		return "", err
	}
	s.rng.Shuffle(len(sws), func(i, j int) { sws[i], sws[j] = sws[j], sws[i] })
	for _, sw := range sws {
		for _, pu := range sw.Ports {
			if vif, ok := vifByUUID[pu]; ok {
				ls := &nb.LogicalSwitch{UUID: sw.UUID}
				ops := must(s.c.Where(ls).Mutate(ls, model.Mutation{
					Field:   &ls.Ports,
					Mutator: ovsdb.MutateOperationDelete,
					Value:   []string{pu},
				}))
				if err := transact(ctx, s.c, ops); err != nil {
					return "", err
				}
				return fmt.Sprintf("remove port %s from %s", vif.Name, sw.Name), nil
			}
		}
	}
	return "skip port.remove (no removable ports)", nil
}

func (s *Simulator) togglePort(ctx context.Context) (string, error) {
	vif, ok, err := s.randomVIF(ctx)
	if err != nil || !ok {
		return "skip port.toggle (no vifs)", err
	}
	newVal := true
	if vif.Enabled != nil {
		newVal = !*vif.Enabled
	}
	lsp := &nb.LogicalSwitchPort{UUID: vif.UUID, Enabled: ptr(newVal)}
	ops, err := s.c.Where(lsp).Update(lsp, &lsp.Enabled)
	if err != nil {
		return "", err
	}
	if err := transact(ctx, s.c, ops); err != nil {
		return "", err
	}
	state := "up"
	if !newVal {
		state = "down"
	}
	return fmt.Sprintf("set port %s admin %s", vif.Name, state), nil
}

// --- NAT ------------------------------------------------------------------

func (s *Simulator) addNAT(ctx context.Context) (string, error) {
	rs, err := listOwned(ctx, s.c, func(r *nb.LogicalRouter) map[string]string { return r.ExternalIDs })
	if err != nil {
		return "", err
	}
	if len(rs) == 0 {
		return "skip nat.add (no routers)", nil
	}
	r := rs[s.rng.Intn(len(rs))]
	seq := s.nextSeq()
	ext := fmt.Sprintf("192.0.2.%d", 50+(seq%150))
	t := newTxn(s.c)
	natUUID := t.namedUUID()
	t.add(&nb.NAT{
		UUID:        natUUID,
		Type:        nb.NATTypeDNATAndSNAT,
		ExternalIP:  ext,
		LogicalIP:   fmt.Sprintf("10.10.0.%d", 100+(seq%150)),
		ExternalIDs: ownedIDs("nat"),
	})
	lr := &nb.LogicalRouter{UUID: r.UUID}
	t.addOps(s.c.Where(lr).Mutate(lr, model.Mutation{
		Field:   &lr.Nat,
		Mutator: ovsdb.MutateOperationInsert,
		Value:   []string{natUUID},
	}))
	if err := t.commit(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("add NAT %s on %s", ext, r.Name), nil
}

func (s *Simulator) removeNAT(ctx context.Context) (string, error) {
	rs, err := listOwned(ctx, s.c, func(r *nb.LogicalRouter) map[string]string { return r.ExternalIDs })
	if err != nil {
		return "", err
	}
	s.rng.Shuffle(len(rs), func(i, j int) { rs[i], rs[j] = rs[j], rs[i] })
	for _, r := range rs {
		if len(r.Nat) == 0 {
			continue
		}
		natUUID := r.Nat[s.rng.Intn(len(r.Nat))]
		lr := &nb.LogicalRouter{UUID: r.UUID}
		ops := must(s.c.Where(lr).Mutate(lr, model.Mutation{
			Field:   &lr.Nat,
			Mutator: ovsdb.MutateOperationDelete,
			Value:   []string{natUUID},
		}))
		if err := transact(ctx, s.c, ops); err != nil {
			return "", err
		}
		return "remove a NAT from " + r.Name, nil
	}
	return "skip nat.remove (no NAT rules)", nil
}

// --- ACL & load balancer --------------------------------------------------

func (s *Simulator) toggleACL(ctx context.Context) (string, error) {
	acls, err := listOwned(ctx, s.c, func(a *nb.ACL) map[string]string { return a.ExternalIDs })
	if err != nil {
		return "", err
	}
	if len(acls) == 0 {
		return "skip acl.toggle (no ACLs)", nil
	}
	a := acls[s.rng.Intn(len(acls))]
	newAction := nb.ACLActionDrop
	if a.Action == nb.ACLActionDrop {
		newAction = nb.ACLActionAllowRelated
	}
	acl := &nb.ACL{UUID: a.UUID, Action: newAction}
	ops, err := s.c.Where(acl).Update(acl, &acl.Action)
	if err != nil {
		return "", err
	}
	if err := transact(ctx, s.c, ops); err != nil {
		return "", err
	}
	return fmt.Sprintf("set ACL %s action=%s", a.UUID[:8], newAction), nil
}

func (s *Simulator) addLBVip(ctx context.Context) (string, error) {
	lbs, err := listOwned(ctx, s.c, func(l *nb.LoadBalancer) map[string]string { return l.ExternalIDs })
	if err != nil {
		return "", err
	}
	if len(lbs) == 0 {
		return "skip lb.vip.add (no load balancers)", nil
	}
	lb := lbs[s.rng.Intn(len(lbs))]
	seq := s.nextSeq()
	vip := fmt.Sprintf("192.0.2.%d:80", 150+(seq%100))
	l := &nb.LoadBalancer{UUID: lb.UUID}
	ops := must(s.c.Where(l).Mutate(l, model.Mutation{
		Field:   &l.Vips,
		Mutator: ovsdb.MutateOperationInsert,
		Value:   map[string]string{vip: "10.10.0.10:8080,10.10.0.11:8080"},
	}))
	if err := transact(ctx, s.c, ops); err != nil {
		return "", err
	}
	return fmt.Sprintf("add VIP %s to %s", vip, lb.Name), nil
}

func (s *Simulator) removeLBVip(ctx context.Context) (string, error) {
	lbs, err := listOwned(ctx, s.c, func(l *nb.LoadBalancer) map[string]string { return l.ExternalIDs })
	if err != nil {
		return "", err
	}
	s.rng.Shuffle(len(lbs), func(i, j int) { lbs[i], lbs[j] = lbs[j], lbs[i] })
	for _, lb := range lbs {
		if len(lb.Vips) <= 1 {
			continue // keep at least the seeded VIP
		}
		var key string
		for k := range lb.Vips {
			key = k
			break
		}
		l := &nb.LoadBalancer{UUID: lb.UUID}
		ops := must(s.c.Where(l).Mutate(l, model.Mutation{
			Field:   &l.Vips,
			Mutator: ovsdb.MutateOperationDelete,
			Value:   []string{key},
		}))
		if err := transact(ctx, s.c, ops); err != nil {
			return "", err
		}
		return fmt.Sprintf("remove VIP %s from %s", key, lb.Name), nil
	}
	return "skip lb.vip.remove (nothing removable)", nil
}

// --- chassis binding (optional) -------------------------------------------

func (s *Simulator) bindPort(ctx context.Context) (string, error) {
	if len(s.binder.Chassis) == 0 {
		return "skip bind (no chassis configured)", nil
	}
	vif, ok, err := s.randomVIFWhere(ctx, func(p nb.LogicalSwitchPort) bool {
		return p.ExternalIDs[boundChassisKey] == ""
	})
	if err != nil || !ok {
		return "skip bind (no unbound vifs)", err
	}
	chassis := s.binder.Chassis[s.rng.Intn(len(s.binder.Chassis))]
	if err := s.binder.Bind(ctx, chassis, vif.Name); err != nil {
		return "", err
	}
	if err := s.setBoundChassis(ctx, vif.UUID, chassis); err != nil {
		return "", err
	}
	return fmt.Sprintf("bind %s onto %s", vif.Name, chassis), nil
}

func (s *Simulator) migratePort(ctx context.Context) (string, error) {
	if len(s.binder.Chassis) < 2 {
		return "skip migrate (need >=2 chassis)", nil
	}
	vif, ok, err := s.randomVIFWhere(ctx, func(p nb.LogicalSwitchPort) bool {
		return p.ExternalIDs[boundChassisKey] != ""
	})
	if err != nil || !ok {
		return "skip migrate (no bound vifs)", err
	}
	from := vif.ExternalIDs[boundChassisKey]
	to := from
	for to == from {
		to = s.binder.Chassis[s.rng.Intn(len(s.binder.Chassis))]
	}
	if err := s.binder.Migrate(ctx, from, to, vif.Name); err != nil {
		return "", err
	}
	if err := s.setBoundChassis(ctx, vif.UUID, to); err != nil {
		return "", err
	}
	return fmt.Sprintf("migrate %s from %s to %s", vif.Name, from, to), nil
}

// setBoundChassis records (or clears, when chassis == "") the chassis a VIF is
// bound to.
func (s *Simulator) setBoundChassis(ctx context.Context, uuid, chassis string) error {
	return recordBoundChassis(ctx, s.c, uuid, chassis)
}

// recordBoundChassis rewrites a VIF's external_ids (selected by UUID) to note
// (or clear, when chassis == "") which chassis it is bound to, preserving the
// ownership markers.
func recordBoundChassis(ctx context.Context, c client.Client, uuid, chassis string) error {
	lsp := &nb.LogicalSwitchPort{UUID: uuid, ExternalIDs: boundIDs(chassis)}
	ops, err := c.Where(lsp).Update(lsp, &lsp.ExternalIDs)
	if err != nil {
		return err
	}
	return transact(ctx, c, ops)
}

// recordBoundChassisByName is like recordBoundChassis but selects the VIF by its
// (indexed) name — used right after creating a port, when its real UUID is not
// yet known locally.
func recordBoundChassisByName(ctx context.Context, c client.Client, name, chassis string) error {
	lsp := &nb.LogicalSwitchPort{Name: name, ExternalIDs: boundIDs(chassis)}
	ops, err := c.Where(lsp).Update(lsp, &lsp.ExternalIDs)
	if err != nil {
		return err
	}
	return transact(ctx, c, ops)
}

// boundIDs returns the external_ids for a VIF, including the bound-chassis marker
// when chassis is non-empty.
func boundIDs(chassis string) map[string]string {
	ids := ownedIDs("vif")
	if chassis != "" {
		ids[boundChassisKey] = chassis
	}
	return ids
}

// --- helpers --------------------------------------------------------------

func (s *Simulator) randomVIF(ctx context.Context) (nb.LogicalSwitchPort, bool, error) {
	return s.randomVIFWhere(ctx, func(nb.LogicalSwitchPort) bool { return true })
}

func (s *Simulator) randomVIFWhere(ctx context.Context, pred func(nb.LogicalSwitchPort) bool) (nb.LogicalSwitchPort, bool, error) {
	ports, err := listOwned(ctx, s.c, func(p *nb.LogicalSwitchPort) map[string]string { return p.ExternalIDs })
	if err != nil {
		return nb.LogicalSwitchPort{}, false, err
	}
	var cand []nb.LogicalSwitchPort
	for _, p := range ports {
		if p.ExternalIDs["nw-kind"] == "vif" && pred(p) {
			cand = append(cand, p)
		}
	}
	if len(cand) == 0 {
		return nb.LogicalSwitchPort{}, false, nil
	}
	return cand[s.rng.Intn(len(cand))], true, nil
}

func switchNames(sws []nb.LogicalSwitch) []string {
	out := make([]string, len(sws))
	for i, s := range sws {
		out[i] = s.Name
	}
	return out
}

func routerNamesOf(rs []nb.LogicalRouter) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

// usedIndices parses the trailing numeric index out of every name that starts
// with prefix (e.g. "nw-ls-" -> 7 from "nw-ls-007").
func usedIndices(names []string, prefix string) map[int]bool {
	used := map[int]bool{}
	for _, n := range names {
		if idx, ok := indexFromName(n, prefix); ok {
			used[idx] = true
		}
	}
	return used
}

func indexFromName(name, prefix string) (int, bool) {
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	rest := name[len(prefix):]
	n, got := 0, false
	for _, ch := range rest {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
		got = true
	}
	return n, got
}

// freeIndex returns the smallest positive integer not present in used.
func freeIndex(used map[int]bool) int {
	for i := 1; ; i++ {
		if !used[i] {
			return i
		}
	}
}

func newSeedResult() *SeedResult { return &SeedResult{Created: map[string]int{}} }

// must panics only on a programming error in building OVSDB ops from a model we
// just constructed; such failures are not recoverable at runtime.
func must(ops []ovsdb.Operation, err error) []ovsdb.Operation {
	if err != nil {
		panic(fmt.Sprintf("ovnsim: building ops: %v", err))
	}
	return ops
}
