# Large deployments

Northwatch keeps the entire NB and SB state in memory, which makes every read
fast but makes the initial load the moment to think about. This page explains
why that moment is heavy and how the tuning knobs bound it. The operational
recipe is in [Tune the initial load](/how-to/tune-the-initial-load).

## Why the first monitor is the heaviest moment

OVSDB has no pagination. A `monitor` (or `monitor_all`) reply contains the
entire initial contents of every monitored table in a single atomic message.
On a large Southbound database, where `Logical_Flow` alone is frequently more
than 90% of all rows, serializing that into one reply can spike memory and CPU on
the `ovsdb-server` producing it, which is often single-threaded. The same applies
to capturing a snapshot, because capture must monitor the live databases first.

After the initial dump, steady-state cost is low: OVSDB sends only the changed
columns of changed rows, so incremental updates are small. The problem is
specifically the one-shot initial transfer.

## Knob 1: stage the monitor over time

`--monitor-batch-delay` (default `200ms`) replaces the single `monitor_all` with
one monitor request per table, spaced by the delay. Each table's initial dump
then arrives as its own smaller reply, spread across time, which bounds peak
memory on both Northwatch and the server.

Staging has one important limitation. It spreads the many tables across time, but
the one dominant table is still delivered as a single reply. Batching does not
chunk within a table, so on a database dominated by `Logical_Flow`, staging helps
the long tail of tables but not the single biggest transfer.

Because NB and SB load concurrently, the added startup time is roughly
`(tables_per_db − 1) × delay`. On a small or dev deployment the staging is pure
overhead, so set `0` to restore the original single-request behaviour.

## Knob 2: skip the dominant tables

`--monitor-skip-tables` is the real lever for the biggest tables: it never
monitors them at all. The trade-off is direct and worth stating plainly. A
skipped table is simply absent from the cache, so every feature that reads it goes
empty:

- `Logical_Flow` → no logical-flow view, no flow trace, no flow diff, no flow
  telemetry.
- `MAC_Binding` / `FDB` → no MAC / forwarding-database views.

For a very large deployment that does not need flow-level debugging from this
instance, skipping `Logical_Flow` is the single most effective reduction.

## Push load off the primary

The two knobs bound the load; where it lands is a separate lever. Pointing
Northwatch at an OVSDB relay or a standby/follower rather than the Raft
leader keeps the initial-dump cost off the database doing the real work. The
comma-separated `--ovn-sb-addr` already accepts multiple endpoints for this. See
[Connect to a Raft cluster](/how-to/connect-to-a-raft-cluster).

## Offline replay is exempt

Serving a snapshot file (`--snapshot`) connects to local in-memory servers that
hold a copy with nothing to protect, so it ignores both knobs and always loads in
a single request. Only live connections are tuned: server startup and snapshot
capture.

## Related

- [Tune the initial load](/how-to/tune-the-initial-load)
- [Architecture](/explanation/architecture)
