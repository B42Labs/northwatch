package ovnsim

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
	"github.com/ovn-kubernetes/libovsdb/model"
	"github.com/ovn-kubernetes/libovsdb/ovsdb"
)

// Clean deletes every simulator-owned object. Deleting the root tables
// (Logical_Switch, Logical_Router, ...) lets OVSDB garbage-collect their
// non-root children (ports, NAT, ACLs, ...). It returns the number of root rows
// removed.
//
// Only rows carrying the ownership marker (see SimTag) are touched, so a
// database shared with other tooling keeps everything else intact. The
// Load_Balancer_Group has no external_ids column, so it is matched by NamePrefix.
func Clean(ctx context.Context, c client.Client) (int, error) {
	total := 0
	steps := []func() (int, error){
		cleanTable[nb.LogicalSwitch](ctx, c),
		cleanTable[nb.LogicalRouter](ctx, c),
		cleanTable[nb.LoadBalancer](ctx, c),
		cleanTable[nb.PortGroup](ctx, c),
		cleanTable[nb.AddressSet](ctx, c),
		cleanTable[nb.Meter](ctx, c),
		cleanTable[nb.DHCPOptions](ctx, c),
		cleanTable[nb.DNS](ctx, c),
		cleanTable[nb.Copp](ctx, c),
		cleanTable[nb.HAChassisGroup](ctx, c),
		func() (int, error) {
			// Load_Balancer_Group has no external_ids; match by name prefix.
			var rows []nb.LoadBalancerGroup
			if err := c.List(ctx, &rows); err != nil {
				return 0, fmt.Errorf("listing load balancer groups: %w", err)
			}
			var u []string
			for i := range rows {
				if strings.HasPrefix(rows[i].Name, NamePrefix) {
					u = append(u, rows[i].UUID)
				}
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.LoadBalancerGroup{UUID: id}).Delete())
			})
		},
	}

	for _, step := range steps {
		n, err := step()
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// cleanTable returns a step that deletes every simulator-owned row of table T.
// Ownership is read from the row's ExternalIDs field and the delete is keyed by
// its UUID; both are accessed via reflection because Go generics cannot name a
// struct field, but every generated NB model carries `ExternalIDs` and `UUID`.
func cleanTable[T any](ctx context.Context, c client.Client) func() (int, error) {
	return func() (int, error) {
		var rows []T
		if err := c.List(ctx, &rows); err != nil {
			return 0, fmt.Errorf("listing %T: %w", *new(T), err)
		}
		n := 0
		for i := range rows {
			extIDs, _ := reflect.ValueOf(rows[i]).FieldByName("ExternalIDs").Interface().(map[string]string)
			if !owned(extIDs) {
				continue
			}
			// Build a minimal &T{UUID: ...} so Where matches by UUID, mirroring
			// the original per-type delete.
			del := new(T)
			reflect.ValueOf(del).Elem().FieldByName("UUID").SetString(
				reflect.ValueOf(rows[i]).FieldByName("UUID").String())
			m, ok := any(del).(model.Model)
			if !ok {
				return n, fmt.Errorf("%T is not an OVSDB model", *new(T))
			}
			ops, err := c.Where(m).Delete()
			if err != nil {
				return n, fmt.Errorf("building delete for %T: %w", *new(T), err)
			}
			if err := transact(ctx, c, ops); err != nil {
				return n, fmt.Errorf("deleting owned %T: %w", *new(T), err)
			}
			n++
		}
		return n, nil
	}
}

// deleteEach transacts a delete for every uuid and returns how many succeeded.
func deleteEach(ctx context.Context, c client.Client, uuids []string, del func(string) []ovsdb.Operation) (int, error) {
	n := 0
	for _, u := range uuids {
		if err := transact(ctx, c, del(u)); err != nil {
			return n, fmt.Errorf("deleting row %s: %w", u, err)
		}
		n++
	}
	return n, nil
}
