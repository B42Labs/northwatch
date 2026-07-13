# History & snapshots

The live cache answers "what is true now?" Northwatch also answers "what changed,
and what did it look like then?" — through a SQLite-backed history store and two
related snapshot mechanisms. This page explains how they fit together and why the
offline mode is designed the way it is.

## The history store

A running server records into an embedded SQLite database (`--history-db-path`):

- **Events** — the stream of OVSDB changes, retained by time
  (`--event-retention`, default `24h`) and optionally capped by count
  (`--event-max-count`).
- **Periodic snapshots** — the full NB+SB state, captured at a fixed interval
  (`--snapshot-interval`, default `5m`), plus on demand.

Events are pruned by age and (optionally) by count. Auto-snapshots are pruned by
count: Northwatch keeps the newest `--snapshot-max-count` (default `500`, `0`
disables) and deletes the rest. Snapshots you took deliberately — manual, labeled
or imported ones — are exempt, because retention must never reclaim something an
operator chose to keep; remove those through the history API
(`DELETE /api/v1/snapshots/{id}`).

SQLite is embedded (`modernc.org/sqlite`, pure Go), so there is no external
database to operate — history travels with the binary.

Two snapshots can be **diffed** to see what changed between two points in time,
and any stored snapshot can be **exported** to a JSON file or **loaded** back at
runtime.

## Two kinds of snapshot

It is worth separating two things that both say "snapshot":

1. **Stored snapshots** live inside the history database of a running server. They
   are what the timeline, diff and runtime-load features operate on.
2. **File snapshots** are standalone files produced by the `northwatch snapshot`
   subcommand (or by exporting a stored one). They are portable — you can carry a
   deployment's state to another machine.

Both capture the same thing (full NB+SB contents); they differ in where they live
and how you get them.

## Loading a snapshot at runtime

A stored snapshot can be loaded into a running server **without a restart**. The
server starts local in-memory OVSDB servers from the snapshot, attaches it as an
additional read-only cluster through the same cluster proxy used for
multi-cluster, and suspends the live OVN monitors while it is loaded (resuming
them on eject). This is why the snapshot becomes browsable instantly and why the
live view pauses rather than competing with it.

While a snapshot session is loaded, suspending the live monitors purges the
default cluster's caches. To keep this from corrupting monitoring data, the
subsystems bound to those caches are paused for the duration:

- **Auto-snapshots** and **event persistence** are skipped, so no empty "auto"
  history snapshot is recorded and the re-population events emitted on resume are
  not persisted.
- **Alert evaluation** is short-circuited, so the 100% cache drop does not fire
  `flow_count_anomaly` (or its webhook) and active alerts are preserved.
- **Flow-diff** collection drops events, so the purge and re-add are not recorded
  as a flow anomaly.

`/readyz` reflects the suspension by returning `503` while a snapshot is loaded,
and live data responses carry an `X-Northwatch-Stale: true` header. Note that a
**manual** `POST /api/v1/history/snapshots` and the write-engine preview still
run against the purged cache while suspended — writing or capturing while a
snapshot session is loaded is an operator error.

Loaded sessions are capped (default 4; a further load returns `409`) because each
keeps two in-memory OVSDB servers and full client caches resident until unloaded.
On unload the backing servers are kept alive for a short drain grace so in-flight
requests finish before their state is torn down. A capture from a churning
production database can reference a row deleted between the per-table reads; such
dangling references are pruned on load rather than failing the load wholesale.

## Offline replay

The standalone offline mode (`--snapshot <file>`) reuses the exact same idea at
process start: it loads the file, serves it from in-memory OVSDB servers, and runs
every read/debug subsystem against that copy — no live OVN connection at all. See
[Explore a deployment offline](/tutorials/explore-a-deployment-offline).

Because a snapshot is a frozen point in time, the **live-tracking subsystems are
deliberately omitted** for a snapshot cluster: alerts, telemetry, flow diff and
the propagation tracker have nothing to track on static data and would only add
background load. Browsing, correlation, search, topology, tracing and export all
work exactly as they do live. This is also why offline mode reports
`mode: snapshot` and the live capabilities, while their subsystems stay quiet.

## A note on initial load

Capturing a snapshot is a live OVSDB monitor, so it is subject to the same
heaviest-moment concern as server startup, and honours the same
`--monitor-batch-delay` / `--monitor-skip-tables` tuning. Offline *replay*,
serving a local copy, ignores them. See [Large
deployments](/explanation/large-deployments).

## Related

- [Capture & serve a snapshot](/how-to/capture-and-serve-a-snapshot)
- [Explore a deployment offline](/tutorials/explore-a-deployment-offline)
