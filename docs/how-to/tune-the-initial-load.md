# Tune the initial load

Building the initial cache is the single heaviest moment for the OVN databases.
An OVSDB `monitor` reply contains the **entire** initial state of a monitored
table in one atomic message — OVSDB has no pagination. On a large Southbound
database (where `Logical_Flow` is often more than 90% of all rows), serializing
that into one reply can spike memory and CPU on the (frequently single-threaded)
`ovsdb-server`.

Two flags bound this load. Both apply to live server mode **and** snapshot
capture; offline replay (`--snapshot <file>`) ignores them, because it serves a
local in-memory copy with nothing to protect. For the reasoning behind them, see
[Large deployments](/explanation/large-deployments).

## Stage the monitor over time

`--monitor-batch-delay` (default `200ms`) issues one monitor request **per
table** with this delay between them, instead of one giant `monitor_all`. Each
table's initial dump then arrives as its own smaller reply, spread over time,
which bounds peak memory on both sides.

```bash
# Small / dev — fastest startup, single request:
./bin/northwatch --ovn-nb-addr ... --ovn-sb-addr ... --monitor-batch-delay 0

# Default — staged at 200ms (no flag needed)
```

NB and SB load concurrently, so the added startup time is roughly
`(tables_per_db − 1) × delay`. The delay spreads the *many* tables across time,
but the one dominant table is still a single reply — batching does not chunk
*within* a table.

The startup connect deadline scales with this delay (base `30s` plus
`delay × table count`), so a large `--monitor-batch-delay` — e.g. `1s` on a huge
database — no longer risks a fixed timeout aborting startup mid-load.

## Skip the dominant tables entirely

`--monitor-skip-tables` never monitors the listed tables at all. This is the real
lever for the biggest Southbound tables. Skipping a table leaves the features
that read it empty:

| Skipped table | What goes empty |
|---|---|
| `Logical_Flow` | Logical-flows view, flow trace, flow diff, flow telemetry |
| `MAC_Binding` | MAC bindings view |
| `FDB` | Forwarding-database view |

```bash
# Very large (e.g. >10k chassis):
./bin/northwatch --ovn-nb-addr ... --ovn-sb-addr ... \
  --monitor-batch-delay 200ms \
  --monitor-skip-tables Logical_Flow
```

## Recommended starting points

| Deployment | Suggested flags |
|---|---|
| Small / dev / test | `--monitor-batch-delay 0` |
| Medium (default) | *no change* — staged at `200ms` |
| Very large | `--monitor-batch-delay 200ms --monitor-skip-tables Logical_Flow` |

For the lowest possible impact on the primary database, also point Northwatch at
an OVSDB relay or a standby/follower rather than the Raft leader — the
comma-separated `--ovn-sb-addr` already supports multiple endpoints. See
[Connect to a Raft cluster](/how-to/connect-to-a-raft-cluster).

## Capturing snapshots

The same flags apply to `northwatch snapshot`, since capture must monitor the
live databases first:

```bash
./bin/northwatch snapshot --ovn-nb-addr ... --ovn-sb-addr ... \
  --monitor-batch-delay 200ms --monitor-skip-tables Logical_Flow \
  --output prod.snapshot.json
```
