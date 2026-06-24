# Project structure

The repository is a single Go module (`github.com/b42labs/northwatch`) plus an
embedded Svelte UI and a self-contained OVN lab.

```
northwatch/
  cmd/
    northwatch/        # server entry point (+ the `snapshot` subcommand)
    ovnsim/            # OVN load generator for the lab (seed/run/bind/clean)
    openapi-export/    # writes the OpenAPI spec to stdout
  internal/
    config/            # CLI flags + env vars + JSON multi-cluster config file
    ovsdb/             # libovsdb client, connection & monitor management
      nb/              # generated Northbound models (+ pinned schema)
      sb/              # generated Southbound models (+ pinned schema)
    cluster/           # multi-cluster registry
    api/               # HTTP server, JSON helpers
      handler/         # route handlers (one file per feature area)
    search/            # Omnisearch engine across NB/SB tables
    correlate/         # NB↔SB entity correlation
    enrich/            # enrichment providers (OpenStack, Kubernetes) + cache
    events/            # OVSDB-change event hub / bridge
    debug/             # connectivity, port diagnostics, ACL audit, stale detection
    gateway/           # gateway / HA-chassis health
    router/            # routing helpers (next-hop, static routes)
    impact/            # impact analysis for an entity
    flowdiff/          # real-time logical-flow diffing
    telemetry/         # Prometheus collector, propagation tracking
    alert/             # alert engine, rules, webhook notifier
    history/           # SQLite event log + periodic snapshots
    snapshot/          # snapshot capture / serve (offline mode)
    snapshotsession/   # load a stored snapshot as a runtime cluster
    write/             # write engine: plans, preview, audit, rate limit
    openapi/           # OpenAPI 3.1 spec builder
    testutil/          # shared test helpers
  ui/
    frontend/          # Svelte + TypeScript SPA
    *.go               # embed.FS for the built assets
  lab/                 # containerlab + Docker Compose OVN lab
  Makefile
  go.mod / go.sum
```

## Conventions

These are summarized from `CLAUDE.md`:

- **libovsdb is the cache.** `MonitorAll` populates an in-memory `TableCache`;
  handlers query it via `client.List()` / `client.Get()` / `client.WhereCache()`.
  There is no custom cache layer.
- **stdlib HTTP.** `net/http` with `http.ServeMux` pattern routing (Go 1.22+). No
  framework.
- **Config is flags + env vars only.** No YAML for the process itself.
- **Tests** use `testify`, table-driven subtests, and hand-written
  function-field mocks (no mockgen). Run them with `go test -race ./...`.

## Generated code

The `internal/ovsdb/nb` and `internal/ovsdb/sb` packages are generated from the
OVN schemas with `make generate`, against schemas pinned by `OVN_VERSION` in the
`Makefile` and refreshed with `make schema-download`.

## Related

- [Make targets](/reference/make-targets)
- [Architecture](/explanation/architecture)
