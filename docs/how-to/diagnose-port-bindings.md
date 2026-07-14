# Diagnose port bindings

A logical port that never realizes on a chassis is one of the most common OVN
problems. Northwatch's debug endpoints make the broken link explicit: a
Northbound `Logical_Switch_Port` exists (intent) but the Southbound `Port_Binding`
has no chassis (no realization).

These endpoints are part of the `debug` capability, which is on by default.

## Find unhealthy ports

The port-diagnostics endpoint runs per-port checks across **all** logical ports
— unbound VIFs, binding mismatches, down-but-bound interfaces — and reports each
port with its check messages and an `overall` severity, plus summary counts:

```bash
curl -s http://localhost:8080/api/v1/debug/port-diagnostics
```

The list accepts `?severity=healthy|warning|error` to filter and `?limit=<n>` to
cap the returned `ports` (the summary counts stay totals); an invalid value
returns `400`. Diagnose a single port by UUID:

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

See the parameter names in the interactive API reference at
<http://localhost:8080/api/v1/docs>.

## Check next-hop MAC resolution

For routing problems, the nexthop-mac report correlates every static route's
next hop with its cached `MAC_Binding` state and flags the conditions that can
blackhole traffic: a learned MAC with no aging configured (never refreshed), an
entry older than the configured aging threshold, a static binding that
contradicts the learned MAC, and next hops with no binding at all. It takes no
parameters — it is a fleet-wide health report, not a per-destination lookup:

```bash
curl -s http://localhost:8080/api/v1/debug/nexthop-mac
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
