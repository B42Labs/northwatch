# Northwatch — Project Conventions

## What is this?

Northwatch is a Go service that connects to OVN Northbound and Southbound OVSDB databases and provides a REST API — plus an embedded Svelte web UI — for browsing, debugging, and monitoring OVN deployments. Most routes are read-only; an opt-in write API (`--write-enabled`) performs guarded mutations against OVN NB.

## Build & Test

```bash
make build          # Build binary to bin/northwatch
make test           # Run all tests with race detector
make lint           # Run golangci-lint
make generate       # Regenerate OVSDB models from schemas
make schema-download # Download pinned OVN schemas
```

## Architecture

- **libovsdb is the cache**: `MonitorAll` populates an in-memory `TableCache`. API handlers query it directly via `client.List()` / `client.Get()` / `client.WhereCache()`. No custom cache layer.
- **stdlib HTTP**: `net/http` with `http.ServeMux` (Go 1.22+ pattern routing). No framework.
- **Config**: flags + env vars only. No YAML.

## Code Style

- Follow standard Go conventions (gofmt, go vet)
- Use `testify/assert` and `testify/require` for tests — no BDD frameworks
- Table-driven tests with `t.Run()` subtests
- Hand-written mocks using function-field pattern — no mockgen
- Errors: `fmt.Errorf("context: %w", err)` wrapping
- No unnecessary abstractions — prefer concrete types over interfaces until testing demands otherwise

## Project Layout

`cmd/`:

- `cmd/northwatch/` — server entry point (+ the `snapshot` subcommand)
- `cmd/ovnsim/` — OVN load generator for the lab (seed/run/bind/unbind/clean)
- `cmd/openapi-export/` — writes the OpenAPI spec to stdout

`internal/`:

- `config/` — CLI flags + env vars + JSON multi-cluster config file
- `ovsdb/` — libovsdb client, connection & monitor management, per-chassis
  OVS connection pool; `nb/`, `sb/`, `vs/` hold the generated models
- `api/` — HTTP server + JSON response helpers; `api/handler/` — route
  handlers (one file per feature area)
- `cluster/` — multi-cluster registry
- `search/` — cross-database search engine
- `correlate/` — NB↔SB entity correlation
- `inventory/` — aggregated SB chassis inventory + liveness
- `enrich/` — enrichment providers (OpenStack, Kubernetes) + cache
- `events/` — OVSDB-change event hub / bridge
- `debug/` — connectivity, port diagnostics, ACL audit, stale detection
- `gateway/` — gateway / HA-chassis health
- `router/` — routing helpers (next-hop, static routes)
- `severity/` — shared healthy/warning/error status vocabulary
- `impact/` — impact analysis for an entity
- `flowdiff/` — real-time logical-flow diffing
- `telemetry/` — Prometheus collector, propagation tracking
- `alert/` — alert engine, rules, webhook notifier
- `history/` — SQLite event log + periodic snapshots
- `snapshot/` — snapshot capture / serve (offline mode)
- `snapshotsession/` — load a stored snapshot as a runtime cluster
- `write/` — write engine: plans, preview, audit, rate limit
- `ovscorrelate/` — correlate live OVS state with OVN intent
- `ovshealth/` — fleet-wide OVS health aggregation
- `openapi/` — OpenAPI 3.1 spec builder
- `ovnsim/` — OVN lab load-generator implementation
- `testutil/` — shared test helpers
- `ui/` — `embed.FS` for the built assets (`ui/frontend/` is the Svelte +
  TypeScript SPA)

## API Routes

Read routes follow `GET /api/v1/{db}/{table}` for list and
`GET /api/v1/{db}/{table}/{uuid}` for detail. Mutating routes also exist: the
write API under `/api/v1/write/*` (including failover/evacuate/restore),
registered only with `--write-enabled`; and alert rule/silence mutations,
history operations, and snapshot create/delete/import/load/unload, registered
unconditionally.

## Testing

- All tests: `go test -race ./...` (also run in CI).
- OVSDB integration tests use libovsdb's in-memory test server — no real OVN
  needed. Each package uses **one shared server per DB kind**, started lazily by
  `internal/testutil` and torn down by a per-package `TestMain` that calls
  `testutil.Main(m)`. `SetupNBTestClient` / `SetupSBTestClient` wipe every table
  on a fresh client before it monitors, so each test starts from an empty
  database. Do not boot per-test servers; do not duplicate the boot helpers.
- OVSDB-backed tests must stay **serial** — the shared server plus wipe-per-test
  isolation makes intra-package parallelism unsafe, so do not add `t.Parallel()`
  to them. Pure-logic suites (no OVSDB, no shared mutable state) may use
  `t.Parallel()`.
- **No `time.Sleep` for synchronization in tests.** Use an injected clock (an
  unexported `now func() time.Time` seam), a completion channel/`require.Eventually`,
  or a protocol round-trip (e.g. a WebSocket ping/pong barrier) instead.
- Coverage: CI enforces a 70% overall gate **and** a per-package 85% floor for
  the designated core packages (`internal/write`, `internal/debug`,
  `internal/api`, `internal/api/handler`) via `scripts/covcheck.sh`. The
  generated `internal/ovsdb/{nb,sb,vs}` packages are excluded from the coverage
  set. Check locally with `make coverage-check`.
- Frontend gates (in `ui/frontend`): `npm run lint`, `npm run format:check`,
  `npm run check` (svelte-check), and `npm run test` (vitest).
