# Diagnose port bindings

A logical port that never realizes on a chassis is one of the most common OVN
problems. Northwatch's debug endpoints make the broken link explicit: a
Northbound `Logical_Switch_Port` exists (intent) but the Southbound `Port_Binding`
has no chassis (no realization).

These endpoints are part of the `debug` capability, which is on by default.

## Find unbound ports

The port-diagnostics endpoint reports ports that are not bound to any chassis,
along with the likely reason:

```bash
curl -s http://localhost:8080/api/v1/debug/port-diagnostics
```

Diagnose a single port by UUID:

```bash
curl -s http://localhost:8080/api/v1/debug/port-diagnostics/<uuid>
```

In the lab, `make lab-unbind` removes the chassis binding from every seeded VIF
so you can see these light up, and `make lab-bind` clears them again.

## Check connectivity between two ports

The connectivity checker analyses the expected path between two logical ports and
flags blocking ACLs or missing routes:

```bash
curl -s 'http://localhost:8080/api/v1/debug/connectivity?<parameters>'
```

See the parameter names in the Swagger UI at <http://localhost:8080/api/v1/docs>.

## Find the next-hop MAC

For routing problems, resolve the next-hop MAC for a destination:

```bash
curl -s 'http://localhost:8080/api/v1/debug/nexthop-mac?<parameters>'
```

## Detect stale entries

Old `MAC_Binding` / `FDB` rows and orphaned bindings show up here:

```bash
curl -s http://localhost:8080/api/v1/debug/stale-entries
```

## Related

- Find the port first: [Search with Omnisearch](/how-to/search-with-omnisearch).
- The correlation model behind "intent vs realization":
  [Correlation & search](/explanation/correlation-and-search).
- The full debug surface: [Capabilities](/reference/capabilities).
