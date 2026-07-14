# Explore a deployment offline

This tutorial shows you the offline workflow end to end: capture a point-in-time
copy of a live OVN deployment into a single file, then serve that file back as a
fully browsable, read-only Northwatch — with no live OVN connection at all. This
is how you take a deployment home to investigate, attach state to an incident
report, or look at a customer's cluster without touching it.

It assumes the lab from [Getting started](/tutorials/getting-started) is running.

## Step 1: Capture a snapshot

The `snapshot` subcommand connects once to the live Northbound and Southbound
databases, copies their full contents, and writes them to a file:

```bash
./bin/northwatch snapshot \
  --ovn-nb-addr tcp:127.0.0.1:6641 \
  --ovn-sb-addr tcp:127.0.0.1:6642 \
  --output lab.snapshot.json
```

Northwatch reports what it captured:

```
Connecting to OVN databases...
Connected; capturing snapshot...
Snapshot written to lab.snapshot.json (NB: 412 rows, SB: 1530 rows)
```

The file records both databases and the source addresses it was taken from. It
is the same staged-monitor capture the server uses at startup, so capturing from
a large deployment is bounded the same way — see
[Tune the initial load](/how-to/tune-the-initial-load).

## Step 2: Serve the snapshot offline

Now start Northwatch in offline mode by pointing `--snapshot` at the file. No
`--ovn-nb-addr` or `--ovn-sb-addr` is needed — Northwatch spins up local
in-memory OVSDB servers from the file and runs every browsing subsystem against
that copy:

```bash
./bin/northwatch --snapshot lab.snapshot.json
```

```
level=INFO msg="loading snapshot" file=lab.snapshot.json
level=INFO msg="snapshot loaded, serving offline copy" nb_rows=412 sb_rows=1530
level=INFO msg="northwatch listening" addr=127.0.0.1:8080 scheme=http
```

## Step 3: Confirm you are offline

Open <http://localhost:8080> — it looks and behaves like the live UI, but it is a
static point in time. Ask the API what mode it is in:

```bash
curl -s http://localhost:8080/api/v1/capabilities
```

```json
{ "capabilities": ["read", "debug", "...", "snapshot"], "mode": "snapshot" }
```

The `mode` is `snapshot` and a `snapshot` capability is present, which is how the
UI shows a "snapshot" indicator instead of "live". Live-only subsystems (alerts,
telemetry, flow diff, the websocket stream) are intentionally absent — a snapshot
is frozen, so there is nothing for them to track. Browsing, correlation, search,
topology, tracing and export all work exactly as they do live.

## Step 4: Browse and trace as usual

Everything read-only behaves the same. Search the offline copy:

```bash
curl -s 'http://localhost:8080/api/v1/search?q=web'
```

…and walk the same correlation chains from
[Investigate with Omnisearch](/tutorials/investigate-with-omnisearch). Because
the data is a frozen copy, you can study a tricky state for as long as you like
without it changing under you.

## What you learned

- `northwatch snapshot` captures both OVN databases to one portable file.
- `northwatch --snapshot <file>` serves that file as a read-only deployment with
  no live connection.
- Offline mode keeps the read/debug surface and drops the live-tracking
  subsystems by design — see
  [History & snapshots](/explanation/history-and-snapshots).

## Next steps

- Capture snapshots on a schedule from a running server and diff them:
  [Capture & serve a snapshot](/how-to/capture-and-serve-a-snapshot).
- Understand the snapshot store and timeline:
  [History & snapshots](/explanation/history-and-snapshots).
