package ovnsim

import (
	"context"
	"strings"

	"github.com/b42labs/northwatch/internal/ovsdb/nb"
	"github.com/ovn-kubernetes/libovsdb/client"
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
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.LogicalSwitch) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.LogicalSwitch{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.LogicalRouter) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.LogicalRouter{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.LoadBalancer) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.LoadBalancer{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.PortGroup) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.PortGroup{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.AddressSet) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.AddressSet{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.Meter) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.Meter{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.DHCPOptions) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.DHCPOptions{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.DNS) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.DNS{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			u, err := ownedUUIDs(ctx, c, func(r *nb.Copp) (map[string]string, string) { return r.ExternalIDs, r.UUID })
			if err != nil {
				return 0, err
			}
			return deleteEach(ctx, c, u, func(id string) []ovsdb.Operation {
				return must(c.Where(&nb.Copp{UUID: id}).Delete())
			})
		},
		func() (int, error) {
			// Load_Balancer_Group has no external_ids; match by name prefix.
			var rows []nb.LoadBalancerGroup
			if err := c.List(ctx, &rows); err != nil {
				return 0, err
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

// deleteEach transacts a delete for every uuid and returns how many succeeded.
func deleteEach(ctx context.Context, c client.Client, uuids []string, del func(string) []ovsdb.Operation) (int, error) {
	n := 0
	for _, u := range uuids {
		if err := transact(ctx, c, del(u)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ownedUUIDs returns the UUIDs of all simulator-owned rows of table T.
func ownedUUIDs[T any](ctx context.Context, c client.Client, get func(*T) (map[string]string, string)) ([]string, error) {
	var rows []T
	if err := c.List(ctx, &rows); err != nil {
		return nil, err
	}
	var out []string
	for i := range rows {
		ids, uuid := get(&rows[i])
		if owned(ids) {
			out = append(out, uuid)
		}
	}
	return out, nil
}
