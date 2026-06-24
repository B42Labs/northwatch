// Package ovnsim is a load generator and topology simulator for OVN
// Northbound. It is used to populate a local OVN lab (see lab/) with realistic
// baseline state so the Northwatch dashboard has something to show, and to
// continuously mutate that state — creating and deleting switches, routers,
// ports, NAT, ACLs and load balancers — so history, events and alerts move.
//
// All objects it creates are tagged with an external_ids marker (see SimTag)
// and named with a common prefix (see NamePrefix). The continuous simulator
// only ever mutates rows carrying that marker, so it never touches objects
// created by anything else sharing the same OVN database.
//
// Writes use the model-based libovsdb API (Create/Update/Delete -> Transact)
// against the generated Northbound models in internal/ovsdb/nb. References
// within a single transaction (a switch and its ports, a router and its
// ports/NAT/routes) are expressed with named UUIDs.
package ovnsim

import (
	"context"
	"fmt"

	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
)

const (
	// SimTag is the external_ids key stamped onto every row ovnsim creates.
	// The continuous simulator filters on it so it only ever mutates its own
	// objects.
	SimTag = "nw-sim"

	// NamePrefix is prepended to every object name ovnsim creates, making its
	// rows easy to spot in the dashboard and in ovn-nbctl output.
	NamePrefix = "nw-"
)

// ownedIDs returns the external_ids map stamped onto a row of the given kind so
// it can later be recognised as simulator-owned.
func ownedIDs(kind string) map[string]string {
	return map[string]string{SimTag: "1", "nw-kind": kind}
}

// owned reports whether a row's external_ids mark it as simulator-owned.
func owned(externalIDs map[string]string) bool {
	return externalIDs[SimTag] != ""
}

// transact runs the operations in a single OVSDB transaction and checks every
// per-operation result, turning an OVSDB-level failure into a Go error. A nil
// or empty op list is a no-op.
func transact(ctx context.Context, c client.Client, ops []ovsdb.Operation) error {
	if len(ops) == 0 {
		return nil
	}
	res, err := c.Transact(ctx, ops...)
	if err != nil {
		return fmt.Errorf("transact: %w", err)
	}
	if _, err := ovsdb.CheckOperationResults(res, ops); err != nil {
		return fmt.Errorf("operation failed: %w", err)
	}
	return nil
}

// txn accumulates Create operations that reference each other by named UUID so
// a whole unit (e.g. a switch and its ports) lands in one atomic transaction.
// The first error from add() is latched and surfaced by commit(), so callers can
// chain many add() calls without checking each one.
type txn struct {
	c   client.Client
	ops []ovsdb.Operation
	n   int
	err error
}

func newTxn(c client.Client) *txn { return &txn{c: c} }

// namedUUID returns a fresh placeholder UUID valid for use as an OVSDB
// named-uuid (a C-identifier: letters, digits, underscore). Set it on a model's
// UUID field before add() so other rows in the same txn can reference it.
func (t *txn) namedUUID() string {
	t.n++
	return fmt.Sprintf("u%d", t.n)
}

// add appends the Create operation(s) for one model to the transaction. A
// previously latched error makes it a no-op.
func (t *txn) add(m model.Model) {
	if t.err != nil {
		return
	}
	ops, err := t.c.Create(m)
	if err != nil {
		t.err = fmt.Errorf("create %T: %w", m, err)
		return
	}
	t.ops = append(t.ops, ops...)
}

// addOps appends pre-built operations (e.g. from a Where().Mutate()/Update())
// to the transaction, latching the first error. Lets create + mutate land
// atomically so named UUIDs resolve across both.
func (t *txn) addOps(ops []ovsdb.Operation, err error) {
	if t.err != nil {
		return
	}
	if err != nil {
		t.err = err
		return
	}
	t.ops = append(t.ops, ops...)
}

// commit transacts the accumulated operations, or returns the latched build
// error if any add() failed.
func (t *txn) commit(ctx context.Context) error {
	if t.err != nil {
		return t.err
	}
	return transact(ctx, t.c, t.ops)
}

// listOwned loads every row of table T whose external_ids mark it as
// simulator-owned, returned via the supplied externalIDs accessor. It reads
// from the libovsdb cache, so the client must be monitoring the database.
func listOwned[T any](ctx context.Context, c client.Client, idsOf func(*T) map[string]string) ([]T, error) {
	var rows []T
	if err := c.List(ctx, &rows); err != nil {
		return nil, fmt.Errorf("listing %T: %w", rows, err)
	}
	out := rows[:0]
	for i := range rows {
		if owned(idsOf(&rows[i])) {
			out = append(out, rows[i])
		}
	}
	return out, nil
}

// hasOwned reports whether at least one simulator-owned row of table T exists.
// Used to make seeding of name-less extra tables (DNS, BFD, ...) idempotent.
func hasOwned[T any](ctx context.Context, c client.Client, idsOf func(*T) map[string]string) (bool, error) {
	rows, err := listOwned[T](ctx, c, idsOf)
	return len(rows) > 0, err
}

// nameSet loads every row of table T and returns the set of names already
// present, via the supplied name accessor. Used to make seeding idempotent
// (OVN NB does not enforce name uniqueness at the database level).
func nameSet[T any](ctx context.Context, c client.Client, nameOf func(*T) string) (map[string]struct{}, error) {
	var rows []T
	if err := c.List(ctx, &rows); err != nil {
		return nil, fmt.Errorf("listing %T: %w", rows, err)
	}
	set := make(map[string]struct{}, len(rows))
	for i := range rows {
		set[nameOf(&rows[i])] = struct{}{}
	}
	return set, nil
}

// mac returns a deterministic, locally-administered unicast MAC from a seed.
func mac(seed int) string {
	return fmt.Sprintf("02:00:00:%02x:%02x:%02x", (seed>>16)&0xff, (seed>>8)&0xff, seed&0xff)
}

func ptr[T any](v T) *T { return &v }
