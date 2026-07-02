# Capture & serve a snapshot

Northwatch has two distinct snapshot mechanisms:

1. A **file snapshot** taken with the `snapshot` subcommand and served offline
   with `--snapshot`. This is the portable, take-it-home copy.
2. **Stored snapshots** captured automatically by a running server into its
   SQLite history database, which you can diff, export, and load at runtime.

This guide covers both. For a guided walkthrough of the file workflow, see
[Explore a deployment offline](/tutorials/explore-a-deployment-offline).

## Capture a file snapshot

Connect once to the live databases and write both to a file:

```bash
./bin/northwatch snapshot \
  --ovn-nb-addr tcp:10.0.0.1:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642 \
  --output prod.snapshot.json
```

Capture is a live monitor, so the initial-load flags apply — see
[Tune the initial load](/how-to/tune-the-initial-load).

## Serve a file snapshot offline

```bash
./bin/northwatch --snapshot prod.snapshot.json
```

No live addresses are needed. The server reports `"mode": "snapshot"` and drops
the live-only subsystems (alerts, telemetry, flow diff, websocket) while keeping
the full read/debug surface.

## Stored snapshots (running server)

A running server periodically snapshots full NB+SB state into its SQLite history
database. Control the cadence and retention with flags:

| Flag | Default | Purpose |
|---|---|---|
| `--history-db-path` | `northwatch-history.db` | SQLite database path |
| `--snapshot-interval` | `5m` | Automatic snapshot interval |
| `--event-retention` | `24h` | How long change events are kept |
| `--event-max-count` | `0` (unlimited) | Cap on retained events |

Work with stored snapshots over the API:

```bash
curl -s http://localhost:8080/api/v1/snapshots                 # list
curl -s -X POST http://localhost:8080/api/v1/snapshots          # take one now
curl -s http://localhost:8080/api/v1/snapshots/<id>            # details
curl -s 'http://localhost:8080/api/v1/snapshots/diff?<params>' # diff two
curl -s http://localhost:8080/api/v1/snapshots/<id>/export     # export to JSON
```

## Load a stored snapshot at runtime

A stored snapshot can be loaded back as a read-only "snapshot" cluster without
restarting the server — it becomes reachable as an additional cluster and the
live OVN connection is suspended while it is loaded:

```bash
curl -s -X POST http://localhost:8080/api/v1/snapshots/<id>/load
curl -s -X POST http://localhost:8080/api/v1/snapshots/<id>/unload
```

While a snapshot is loaded, `/readyz` returns `503` and the history, alert and
flow-diff collectors are paused so the suspended live view does not record bogus
data. Loaded sessions are capped (a further load returns `409`). A capture from a
churning database can reference rows deleted between reads; those dangling
references are pruned on load rather than failing the load. See [History &
snapshots](/explanation/history-and-snapshots#loading-a-snapshot-at-runtime).

You can also import a previously exported snapshot file:

```bash
curl -s -X POST --data-binary @prod.snapshot.json \
  http://localhost:8080/api/v1/snapshots/import
```

## Related

- [History & snapshots](/explanation/history-and-snapshots) — how the store and
  offline replay are designed.
- [CLI flags & env vars](/reference/cli) — every history/snapshot flag.
