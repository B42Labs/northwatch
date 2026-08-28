# Project structure

The repository is a single Go module (`github.com/b42labs/northwatch`) plus an
embedded Svelte UI and a self-contained OVN lab.

```
northwatch/
  cmd/
    northwatch/        # server entry point (+ the `snapshot` subcommand)
    ovnsim/            # OVN load generator for the lab (seed/run/bind/unbind/clean)
    openapi-export/    # writes the OpenAPI spec to stdout
  internal/
    config/            # CLI flags + env vars + JSON multi-cluster config file
    ovsdb/             # libovsdb client, connection & monitor management
                       #   (pool.go: per-chassis OVS connection pool + TLS)
      nb/              # generated Northbound models (+ pinned schema)
      sb/              # generated Southbound models (+ pinned schema)
      vs/              # generated Open_vSwitch models (+ pinned schema)
    cluster/           # multi-cluster registry
    api/               # HTTP server, JSON helpers
      handler/         # route handlers (one file per feature area)
    search/            # Omnisearch engine across NB/SB tables
    correlate/         # NB↔SB entity correlation
    inventory/         # aggregated SB chassis inventory + liveness
    enrich/            # enrichment providers (OpenStack, Kubernetes) + cache
    events/            # OVSDB-change event hub / bridge
    debug/             # connectivity, port diagnostics, ACL audit, stale detection
    gateway/           # gateway / HA-chassis health
    router/            # routing helpers (next-hop, static routes)
    severity/          # shared healthy/warning/error status vocabulary
    impact/            # impact analysis for an entity
    flowdiff/          # real-time logical-flow diffing
    telemetry/         # Prometheus collector, propagation tracking
    alert/             # alert engine, rules, webhook notifier
    history/           # SQLite event log + periodic snapshots
    snapshot/          # snapshot capture / serve (offline mode)
    snapshotsession/   # load a stored snapshot as a runtime cluster
    write/             # write engine: plans, preview, audit, rate limit
    ovscorrelate/      # correlate live per-chassis OVS state with OVN intent
    ovshealth/         # fleet-wide OVS health aggregation
    openapi/           # OpenAPI 3.1 spec builder
    ovnsim/            # OVN lab load-generator implementation (seed/run/bind/unbind/clean)
    testutil/          # shared test helpers
  ui/
    frontend/          # Svelte + TypeScript SPA
    *.go               # embed.FS for the built assets
  lab/                 # containerlab + Docker Compose OVN lab
  packaging/           # Debian packaging inputs: systemd unit, env file, maintainer scripts
  scripts/             # CI helpers (covcheck.sh: the per-package coverage gate)
  contrib/             # extra deployment assets (testbed CA bundle)
  nfpm.yaml            # nfpm spec for the .deb (see make deb / the release workflow)
  Makefile
  go.mod / go.sum
```

## Conventions

These are summarized from `CLAUDE.md`:

- libovsdb is the cache. `MonitorAll` populates an in-memory `TableCache`;
  handlers query it via `client.List()` / `client.Get()` / `client.WhereCache()`.
  There is no custom cache layer.
- stdlib HTTP. `net/http` with `http.ServeMux` pattern routing (Go 1.22+). No
  framework.
- Config is flags + env vars only. No YAML for the process itself.
- Tests use `testify`, table-driven subtests, and hand-written
  function-field mocks (no mockgen). Run them with `go test -race ./...`.

## Generated code

The `internal/ovsdb/nb`, `internal/ovsdb/sb` and `internal/ovsdb/vs` packages
are generated from the OVN and Open_vSwitch schemas with `make generate`,
against schemas pinned by `OVN_VERSION` / `OVS_VERSION` in the `Makefile` and
refreshed with `make schema-download`.

## Related

- [Make targets](/reference/make-targets)
- [Architecture](/explanation/architecture)
