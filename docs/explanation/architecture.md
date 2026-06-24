# Architecture

Northwatch is a single Go binary that connects to the OVN Northbound and
Southbound OVSDB databases, keeps a live in-memory copy of both, and serves a web
UI and REST API over it. This page explains the moving parts and why they are
arranged this way.

## The shape

```
                              +------------------+
                              |   Enrichment     |
                              | (OpenStack, K8s) |
                              +--------+---------+
                                       | name resolution
                                       v
+---------------+   TCP/OVSDB   +------+--------+   HTTP / WS   +-----------+
| OVN Northbound|<------------->|              |<------------->|  Web UI    |
|   Database    |   monitor     |  Northwatch  |               +-----------+
+---------------+               |  in-memory   |
+---------------+   TCP/OVSDB   |  cache +     |   HTTP        +-----------+
| OVN Southbound|<------------->|  event hub   |<------------->| API/Prom. |
|   Database    |   monitor     |  + SQLite    |               +-----------+
+---------------+               +--------------+
                                 single binary
```

## libovsdb is the cache

Northwatch does not build its own datastore. On connect it issues OVSDB `monitor`
requests for every table in both schemas; [libovsdb](https://github.com/ovn-kubernetes/libovsdb)
maintains the resulting rows in an in-memory `TableCache` and applies incremental
updates as they arrive. API handlers query that cache directly with
`client.List()`, `client.Get()` and `client.WhereCache()`. There is deliberately
no second cache layer to keep in sync.

The consequence: **all reads are local and fast**, and the data is always as
fresh as the last OVSDB update. The cost is memory — the entire NB and SB state
lives in RAM — which is acceptable for a dedicated observability service and is
the reason the initial load is tunable (see [Large deployments](/explanation/large-deployments)).

## From change to UI

1. OVN writes to a database; `ovsdb-server` sends a monitor update.
2. libovsdb applies it to the `TableCache`.
3. A small **event hub** bridges cache updates into an internal event stream.
4. Subsystems consume that stream: the WebSocket fan-out pushes it to browsers,
   the alert engine evaluates rules, the flow-diff collector records changes, the
   propagation tracker times config realization, and the history collector logs
   events and takes periodic snapshots.

Reads never wait on any of this — they hit the cache. The event stream is what
makes the UI real-time.

## One binary, embedded everything

- The web UI (Svelte SPA) is compiled and embedded via `go:embed`, so the binary
  serves its own frontend.
- The history/snapshot store is embedded SQLite (`modernc.org/sqlite`, pure Go,
  no CGO), so there is no external database to run.
- HTTP routing is stdlib `net/http` with `http.ServeMux` pattern routing — no web
  framework.

The result is a static binary with no runtime dependencies beyond reachability to
the OVN databases.

## Clusters and sub-muxes

Each configured OVN deployment is a `cluster.Cluster` bundling its own database
clients, correlator, search engine, enricher and live subsystems. The default
cluster is served at the top-level routes; every cluster is also mounted under
`/api/v1/clusters/{name}/...` by a cluster proxy. A loaded snapshot is attached
the same way — as just another cluster — which is why it needs no restart. See
[Monitor multiple clusters](/how-to/monitor-multiple-clusters).

## Offline mode

The same code path runs against a captured snapshot: `--snapshot` starts local
in-memory OVSDB servers from the file and points a cluster at them, so every
read/debug subsystem works unchanged on the offline copy. See
[History & snapshots](/explanation/history-and-snapshots).

## Related

- [Correlation & search](/explanation/correlation-and-search)
- [Project structure](/reference/project-structure)
