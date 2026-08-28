# Roadmap

This page describes where Northwatch is heading. It is intentionally
understanding-oriented: it covers the direction and the known gaps, not a dated
plan. For what works today, the rest of the documentation is the source of truth.

## What exists today

The read, correlate and debug surface is the mature core:

- Live browsing of every NB and SB table, backed by the libovsdb in-memory cache.
- NB↔SB correlation and Omnisearch across both databases.
- Debugging: packet trace, connectivity checks, port diagnostics, ACL audit,
  stale-entry detection, flow diff.
- Topology views (logical, gateway, NAT, load balancer) and a real-time
  WebSocket event stream.
- Telemetry with a Prometheus endpoint, an alert engine with webhook
  notifications, and a SQLite history store with periodic snapshots.
- Multi-cluster monitoring, Raft endpoint failover, and OpenStack / Kubernetes
  enrichment.
- Opt-in per-chassis OVS visibility: live Open_vSwitch state per chassis,
  correlated with OVN intent, plus fleet-wide OVS health.
- Offline mode: capture a deployment to a file and serve it read-only.
- Opt-in write operations behind a plan/preview/apply workflow with an audit log
  and rate limiting, plus operational actions (failover, evacuate, rollback,
  restore).
- Bearer-token authentication on every mutating route, optional HTTPS for the
  API, TLS for the OVN and OVS connections, and a Debian package with a systemd
  unit.

## Direction

The themes below are where the rough edges are.

### Scale

Very large Southbound databases remain the hardest case. The current levers are
the staged monitor and table skipping (see [Large
deployments](/explanation/large-deployments)). The natural next step is
conditional monitoring (`monitor_cond`) so a table can be partially loaded
rather than skipped entirely. That keeps a feature usable without paying for the
full table.

### Tracing fidelity

Evaluating an `ovn-trace`-equivalent purely from cached flow state is inherently
approximate at the edges (conntrack, NAT, load-balancer DNAT). Expect the tracer
to grow more complete over time and to be explicit in the UI about which actions
are fully evaluated versus approximated.

### Hardening

The transport and authentication layer has landed: mutating routes require
bearer tokens, the HTTP surface can serve TLS, the OVSDB connections (OVN NB/SB
and per-chassis OVS) support `ssl:`, and the Debian package ships a systemd
unit. The remaining packaging item is a first-class container image. The read
surface stays unauthenticated by design. When readers are not all trusted, run
Northwatch behind an authenticating reverse proxy; see [The capability
model](/explanation/capability-model).

## How to read this

If a feature is documented in the [Reference](/reference/) or has a
[How-to guide](/how-to/), it exists and is usable. This page is for orientation
about what is solid, what is approximate, and what is still to come.
