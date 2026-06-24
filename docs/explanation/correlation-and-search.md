# Correlation & search

OVN's state is split across two databases, and the hard part of operating it is
holding both in your head at once. Northwatch's central idea is to make that join
a first-class feature: **correlation** links the two databases, and
**Omnisearch** is the entry point that uses those links.

## Intent vs realization

The two OVN databases describe the same network from two angles:

- The **Northbound** database is *intent*. A `Logical_Switch_Port` says "this port
  should exist on this switch." It is what an operator or orchestrator (Neutron,
  ovn-kubernetes) writes.
- The **Southbound** database is *realization*. `ovn-northd` compiles the
  Northbound intent into `Datapath_Binding`, `Logical_Flow` and `Port_Binding`
  rows, and `ovn-controller` on each chassis claims the bindings it can host.

A healthy object lines up on both sides. A problem is usually a place where they
**don't**: intent with no realization (a port that never bound), or realization
pointing nowhere (a binding with no chassis).

## What the correlator does

The correlator walks the references between the databases so you don't have to:

```
Logical_Switch_Port (NB)  →  Port_Binding (SB)  →  Chassis (SB)  →  Encap
Logical_Switch      (NB)  →  Datapath_Binding (SB)  →  Logical_Flows
```

The `correlated/*` endpoints return an entity together with its counterpart on
the other side and any enrichment context, so a single request answers "where did
this end up?" rather than forcing several lookups across 80+ tables. A broken
chain is itself the diagnosis — which is what the [debug
endpoints](/how-to/diagnose-port-bindings) surface explicitly.

## Why search is the entry point

Real investigations start from one fact — an IP a user reported, a MAC from a
packet capture, a UUID from a log. Omnisearch takes that single value and:

1. **Detects the type** — IPv4/IPv6, MAC (full or partial), UUID (full or
   fragment), or free text.
2. **Queries every indexed table in parallel** across both databases (and the
   enrichment caches), because the same value can appear in many places — a flow
   match, a NAT rule, an address set, a port binding.
3. **Groups the results** by entity type so you can pick the one you meant.

From any result you follow the correlation chain above. Search finds it;
correlation explains it.

## Indexed tables

The search engine indexes the highest-value tables rather than every column of
every table — the Northbound logical objects (switches, ports, routers, ACLs,
NAT, address sets, port groups, load balancers, routes, policies, DNS) and the
Southbound realization tables (chassis, port bindings, logical flows, datapath
bindings, encaps, MAC bindings, FDB, address sets, DNS, load balancers).

A table you exclude with `--monitor-skip-tables` is not in the cache and therefore
not searchable — the trade-off described in [Large
deployments](/explanation/large-deployments).

## Related

- [Search with Omnisearch](/how-to/search-with-omnisearch)
- [Investigate with Omnisearch](/tutorials/investigate-with-omnisearch)
- [Enrichment](/explanation/enrichment)
