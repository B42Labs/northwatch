# Northwatch

Northwatch is a real-time analyzer, visualizer, debugger and monitor for
[OVN](https://www.ovn.org/). A single Go binary connects to the OVN Northbound
and Southbound OVSDB databases, keeps a live in-memory copy of both, and serves
one web UI and REST API on top of them — so browsing a table, following a port
from Northbound intent to the chassis that realizes it, tracing a packet, and
watching an alert fire all happen in the same place, against the same data.

There is no agent to install and no database to run alongside it. Northwatch
monitors OVSDB the way `ovn-northd` does, and everything else — correlation,
search, diagnostics, telemetry, history — is derived from that live cache. It
is read-only by default; writing to OVN, enriching UUIDs from OpenStack or
Kubernetes, and reaching out to per-chassis Open vSwitch are separate opt-in
capabilities.

## Start here

- [Getting started](/tutorials/getting-started) — bring up the bundled OVN lab,
  start Northwatch against it, and open the UI on a deployment that has live
  data flowing through it.
- [Install on Debian/Ubuntu](/how-to/install-debian) — the packaged install with
  a systemd unit, for pointing Northwatch at a deployment you already run.
- [Investigate with Omnisearch](/tutorials/investigate-with-omnisearch) — start
  from a single IP address and follow the correlation chain across both
  databases to the chassis hosting the port.

## What's inside

- **Browse and correlate.** Every Northbound and Southbound table, live, plus
  the links between them: `Logical_Switch_Port` → `Port_Binding` → `Chassis`.
  Logical, physical, gateway, NAT and load-balancer topology are rendered from
  the same cache. See [Correlation & search](/explanation/correlation-and-search).
- **Search.** Omnisearch takes an IP, MAC, UUID or name and returns everything
  related to it across both databases, ranked, without you having to know which
  table to look in. See [Search with Omnisearch](/how-to/search-with-omnisearch).
- **Debug.** Packet trace, connectivity checks, port-binding diagnostics, ACL
  audit, stale-entry detection, and real-time logical-flow diffing. Opt in to
  the per-chassis OVS connection pool and Northwatch also correlates live Open
  vSwitch state against OVN intent, with fleet-wide OVS health on top.
- **Monitor.** A Prometheus endpoint, propagation tracking from Northbound
  change to Southbound realization, and an alert engine with rules, silences and
  webhook notifications. See [Prometheus metrics](/reference/metrics) and
  [Configure alerts](/how-to/configure-alerts).
- **History and snapshots.** A SQLite event log and periodic snapshots, with
  diffing and offline replay — capture a deployment and browse it later from
  anywhere. See [History & snapshots](/explanation/history-and-snapshots).
- **Multi-cluster.** Several OVN deployments watched from one process, each with
  its own connections and cache. See
  [Monitor multiple clusters](/how-to/monitor-multiple-clusters).
- **Writes, guarded.** With `--write-enabled`, mutations run through a
  plan/preview/apply workflow with rate limiting and an audit log. See
  [Write safety](/explanation/write-safety).

Every mutating route requires a bearer token, and the API and all OVSDB
connections can run over TLS. What the server exposes is advertised as a list of
[capabilities](/reference/capabilities) the UI reads at startup.

## Go deeper

- [Tutorials](/tutorials/) — learning-oriented, guided paths that take you from
  nothing to a working result in one linear sequence.
- [How-to guides](/how-to/) — task-oriented recipes: connect to a Raft cluster,
  enrich with OpenStack, trace a packet, diagnose a port binding, capture a
  snapshot, enable writes, deploy to production.
- [Reference](/reference/) — every CLI flag and environment variable, the
  config-file schema, the HTTP API surface, the capability list, the Prometheus
  metrics, and the repository layout.
- [Explanation](/explanation/) — the architecture, the NB↔SB correlation model,
  the capability model, enrichment, snapshots, write safety, and the reasoning
  behind initial-load tuning.

The four sections above follow the [Diátaxis](https://diataxis.fr/) framework,
which separates documentation by reader need: learning, tasks, lookup, and
understanding. The source lives on
[GitHub](https://github.com/B42Labs/northwatch).
