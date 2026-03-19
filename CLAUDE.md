# Northwatch — Project Conventions

## What is this?

Northwatch is a Go service that connects to OVN Northbound and Southbound OVSDB databases and provides a REST API for browsing, debugging, and monitoring OVN deployments.

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

- `cmd/northwatch/` — entry point
- `internal/config/` — CLI flags + env var parsing
- `internal/ovsdb/` — OVSDB client connection and model definitions
- `internal/ovsdb/nb/` — generated Northbound models
- `internal/ovsdb/sb/` — generated Southbound models
- `internal/api/` — HTTP server, JSON response helpers
- `internal/api/handler/` — route handlers
- `internal/search/` — cross-database search engine

## API Routes

All routes are read-only. Pattern: `GET /api/v1/{db}/{table}` for list, `GET /api/v1/{db}/{table}/{uuid}` for detail.

## Testing

- All tests: `go test -race ./...`
- OVSDB integration tests use libovsdb's in-memory test server — no real OVN needed
- Coverage target: 70% overall, 85%+ for core packages
