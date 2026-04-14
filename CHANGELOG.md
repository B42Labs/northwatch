# Changelog

All notable changes to Northwatch are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Configurable WebSocket Origin allowlist via `--ws-allowed-origins` /
  `NORTHWATCH_WS_ALLOWED_ORIGINS`. When unset, origin checking is disabled
  (single-tenant deployment default).
- Tests for impact, cluster proxy, lb_topology, nat_topology, flow_tables,
  and uuid handlers.
- Doc comments on previously undocumented exported symbols in `internal/ovsdb`,
  `internal/search`, and `internal/api`.

### Changed
- `write.PlanCache.StartCleanup` now accepts a context and exits when it is
  cancelled. `write.Engine.Start` returns a stop function so the background
  cleanup goroutine no longer leaks at shutdown.
- `write.NewEngine` now returns `(*Engine, error)` instead of panicking on
  `crypto/rand` failure.
- `write.NewAuditStore` accepts a `context.Context` instead of using
  `context.Background()` internally.
- `GET /api/v1/write/plans/{id}` no longer echoes the `apply_token` field.
- `handleTopology` and `handleExportTopology` share a single
  `fetchTopologyData` helper that fetches the seven NB and SB tables in
  parallel via `errgroup`.
- `buildTopology` split into `buildTopologyIndex`, `buildTopologyNodes`,
  `buildTopologyEdges`, and `addVMPorts` for readability.
- `cmd/northwatch/main.go` `run()` extracted into `buildCluster`,
  `registerDefaultRoutes`, and `registerClusterRoutes` helpers.
- `telemetry.Collector` fetches the chassis list once per scrape instead of
  twice, and documents why it has to use `context.Background()`.
- `events.Subscriber` uses `sync.RWMutex` so concurrent publishes can run
  filter checks without serializing on a single mutex.
- `alert.Engine.evaluate` holds the write lock for the entire seen-check-and-
  insert critical section, eliminating a small TOCTOU window.
- `TraceStore.Store` amortizes expired-entry sweeps across many writes.

### Fixed
- `govulncheck` is now a blocking step in CI; previously failures were
  silently ignored via `continue-on-error: true`.
- Avoid relying on backing-array sharing when concatenating snapshot source
  slices in `cmd/northwatch/main.go`.

## [0.1.0] - Initial release

Initial public version with read-only NB/SB API, topology, search, write
operations, history snapshots, alerting, and multi-cluster support.
