# CLI flags & environment variables

Northwatch is configured entirely by command-line flags and environment
variables. There is no YAML config for the process itself (multi-cluster setups
use a JSON [config file](/reference/configuration)). Every flag has an equivalent
environment variable, and flags take precedence over environment variables.

Run `./bin/northwatch --help` to see the same list at runtime.

## Server flags

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--listen` | `NORTHWATCH_LISTEN` | `127.0.0.1:8080` | HTTP listen address. Binding a non-loopback address requires `--api-tokens` or `--insecure-no-auth`; Northwatch refuses to start otherwise. |
| `--ovn-nb-addr` | `NORTHWATCH_OVN_NB_ADDR` | *(required)* | OVN Northbound address. Comma-separated for Raft failover, e.g. `tcp:10.0.0.1:6641,tcp:10.0.0.2:6641`. |
| `--ovn-sb-addr` | `NORTHWATCH_OVN_SB_ADDR` | *(required)* | OVN Southbound address. Comma-separated for Raft failover. |
| `--config-file` | `NORTHWATCH_CONFIG_FILE` | *(none)* | Path to a JSON multi-cluster config file. When set, `--ovn-nb-addr` / `--ovn-sb-addr` are ignored. |
| `--snapshot` | `NORTHWATCH_SNAPSHOT` | *(none)* | Serve a snapshot file offline. Takes precedence over `--ovn-*-addr` and `--config-file`. |
| `--version` | — | — | Print the build version and exit (also accepted as `-version` or bare `version`). Release builds report the release tag; other builds report `dev`. |

Exactly one connection source is required: a snapshot, a config file, or both
flat NB/SB addresses.

## Authentication

Every mutating endpoint (`POST`/`PUT`/`DELETE` under `/api/`) requires a bearer
token. Read endpoints are open; protect those with a reverse proxy, see [Deploy
to production](/how-to/deploy-production).

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--api-tokens` | `NORTHWATCH_API_TOKENS` | *(none)* | Comma-separated `name=token` pairs authorizing mutating requests. The name becomes the write-audit `actor`. Tokens must be at least 16 characters. |
| `--api-tokens-file` | `NORTHWATCH_API_TOKENS_FILE` | *(none)* | Path to a JSON object mapping token name to token (`{"ops": "…"}`), merged with `--api-tokens`. Keeps secrets out of the process table. The file must be a non-empty JSON object; empty names or token values fail startup. |
| `--insecure-no-auth` | `NORTHWATCH_INSECURE_NO_AUTH` | `false` | Disable the token gate entirely. Only safe behind a proxy that authenticates every mutating request; audit entries are then recorded as `anonymous`. Cannot be combined with tokens from `--api-tokens` or `--api-tokens-file`. |

With no tokens configured, mutating endpoints answer `401`, including on
loopback. A token name that appears in both `--api-tokens` and
`--api-tokens-file` is a startup error.

## TLS

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--tls-cert` | `NORTHWATCH_TLS_CERT` | *(none)* | Server certificate (PEM). Serves the API over HTTPS (TLS 1.2 minimum). |
| `--tls-key` | `NORTHWATCH_TLS_KEY` | *(none)* | Server private key (PEM). Must be set together with `--tls-cert`. |
| `--ovn-nb-tls-cert` | `NORTHWATCH_OVN_NB_TLS_CERT` | *(none)* | Client certificate (PEM) for `ssl:` Northbound connections. |
| `--ovn-nb-tls-key` | `NORTHWATCH_OVN_NB_TLS_KEY` | *(none)* | Client private key (PEM) for `ssl:` Northbound connections. |
| `--ovn-nb-tls-ca` | `NORTHWATCH_OVN_NB_TLS_CA` | *(none)* | CA bundle (PEM) verifying `ssl:` Northbound servers. |
| `--ovn-sb-tls-cert` | `NORTHWATCH_OVN_SB_TLS_CERT` | *(none)* | Client certificate (PEM) for `ssl:` Southbound connections. |
| `--ovn-sb-tls-key` | `NORTHWATCH_OVN_SB_TLS_KEY` | *(none)* | Client private key (PEM) for `ssl:` Southbound connections. |
| `--ovn-sb-tls-ca` | `NORTHWATCH_OVN_SB_TLS_CA` | *(none)* | CA bundle (PEM) verifying `ssl:` Southbound servers. |

Each NB/SB trio is all-or-none. An `ssl:` address with no TLS material for that
database is a startup error. The same `--ovn-*-tls-*` flags are accepted by the
`northwatch snapshot` subcommand. The material applies to every cluster in a
`--config-file`; per-cluster material is not supported.

## Initial-load tuning

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--monitor-batch-delay` | `NORTHWATCH_MONITOR_BATCH_DELAY` | `200ms` | Delay between staged per-table monitor requests on connect (Go duration, not negative). `0` loads all tables in one request. Offline `--snapshot` replay always uses `0`. |
| `--monitor-skip-tables` | `NORTHWATCH_MONITOR_SKIP_TABLES` | *(none)* | Comma-separated OVN tables to never monitor (e.g. `Logical_Flow,MAC_Binding,FDB`). Features reading a skipped table see it as empty. |

See [Tune the initial load](/how-to/tune-the-initial-load) and
[Large deployments](/explanation/large-deployments).

## OpenStack enrichment

Setting `--os-auth-url` enables OpenStack enrichment for the default cluster.

| Flag | Env var | Default |
|---|---|---|
| `--os-auth-url` | `OS_AUTH_URL` | *(none)* |
| `--os-username` | `OS_USERNAME` | *(none)* |
| `--os-password` | `OS_PASSWORD` | *(none)* |
| `--os-project-name` | `OS_PROJECT_NAME` | *(none)* |
| `--os-domain-name` | `OS_USER_DOMAIN_NAME` | *(none)* |
| `--os-region-name` | `OS_REGION_NAME` | *(none)* |
| `--os-cacert` | `OS_CACERT` | *(none)* |

`--os-cacert` is a path to a PEM CA bundle used to verify the OpenStack API when
it is fronted by a private CA (maps to the clouds.yaml `cacert` field). The
bundle is trusted only by the OpenStack client, not the whole process.

## Kubernetes enrichment

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--kube-enrichment` | `NORTHWATCH_KUBE_ENRICHMENT` | `false` | Enable Kubernetes enrichment. |
| `--kubeconfig` | `KUBECONFIG` | *(none)* | Path to kubeconfig file. |
| `--kube-context` | `NORTHWATCH_KUBE_CONTEXT` | *(none)* | Kubeconfig context to use. |

## Enrichment cache

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--enrichment-cache-ttl` | `NORTHWATCH_ENRICHMENT_CACHE_TTL` | `5m` | Enrichment cache TTL (Go duration). |

## Chassis inventory

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--chassis-stale-threshold` | `NORTHWATCH_CHASSIS_STALE_THRESHOLD` | `60s` | How long an out-of-sync chassis may lag the current `nb_cfg` generation before the [chassis inventory](/reference/api#chassis-inventory) flags it `stale` (Go duration, must be positive). |

This threshold only affects the `stale` flag, not `alive`/down: a chassis is
alive whenever it is present and in-sync. `nb_cfg_timestamp` only advances when a
chassis acknowledges a new `nb_cfg` generation, so on a steady-state cluster it
freezes, which is why staleness, not timestamp age, is what this bounds. Raise
it to tolerate slower config propagation before a lagging chassis is flagged.
Sync state is derived from `Chassis_Private.nb_cfg` vs `SB_Global.nb_cfg`, which
is accurate on OVN ≥ 20.06.

## OVS visibility (per-chassis Open_vSwitch)

Opt-in, read-only integration with each chassis's local Open_vSwitch (vswitchd)
OVSDB. It is disabled by default and enabled only by pointing
`--ovs-mgmt-addr-file` at a mapping file; the resulting routes are documented
under [OVS](/reference/api#ovs-per-chassis-open_vswitch).

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--ovs-mgmt-addr-file` | `NORTHWATCH_OVS_MGMT_ADDR_FILE` | *(none)* | Path to a JSON file mapping chassis system-id to OVSDB management address. Enables OVS visibility for the default cluster. The file must be a non-empty JSON object; empty system-ids or addresses fail startup. |
| `--ovs-tls-cert` | `NORTHWATCH_OVS_TLS_CERT` | *(none)* | Client certificate (PEM) for `ssl:` OVS connections. |
| `--ovs-tls-key` | `NORTHWATCH_OVS_TLS_KEY` | *(none)* | Client private key (PEM) for `ssl:` OVS connections. |
| `--ovs-tls-ca` | `NORTHWATCH_OVS_TLS_CA` | *(none)* | CA bundle (PEM) verifying `ssl:` OVS servers. |

The mapping file is a JSON object, e.g.:

```json
{
  "chassis-system-id-1": "ssl:10.0.0.11:6640",
  "chassis-system-id-2": "tcp:10.0.0.12:6640"
}
```

Each chassis must enable remote OVSDB access (`ovs-vsctl set-manager
ptcp:6640` or `pssl:6640`). The TLS flags are required only for `ssl:`
endpoints and are all-or-none: supply all three together or none. They are
global flags wired to the default cluster, not part of the multi-cluster JSON
[config file](/reference/configuration).

## Write operations

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--write-enabled` | `NORTHWATCH_WRITE_ENABLED` | `false` | Enable write operations against OVN NB. |
| `--write-plan-ttl` | `NORTHWATCH_WRITE_PLAN_TTL` | `10m` | TTL for prepared write plans (Go duration). |
| `--write-rate-limit` | `NORTHWATCH_WRITE_RATE_LIMIT` | `30` | Maximum write operations per minute (0 = unlimited). |

See [Enable write operations](/how-to/enable-write-operations).

## History & snapshots

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--history-db-path` | `NORTHWATCH_HISTORY_DB_PATH` | `northwatch-history.db` | Path to the SQLite history database. |
| `--snapshot-interval` | `NORTHWATCH_SNAPSHOT_INTERVAL` | `5m` | Automatic snapshot interval (Go duration). |
| `--event-retention` | `NORTHWATCH_EVENT_RETENTION` | `24h` | Event-log retention duration. |
| `--event-max-count` | `NORTHWATCH_EVENT_MAX_COUNT` | `0` | Maximum number of events to retain (0 = unlimited). |
| `--snapshot-max-count` | `NORTHWATCH_SNAPSHOT_MAX_COUNT` | `500` | Maximum number of automatic snapshots to retain (0 = unlimited, negative values fail startup). Labeled, manual and imported snapshots are never pruned. |

## Alerting & WebSocket

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--alert-webhook-urls` | `NORTHWATCH_ALERT_WEBHOOK_URLS` | *(none)* | Comma-separated webhook URLs for alert notifications. |
| `--ws-allowed-origins` | `NORTHWATCH_WS_ALLOWED_ORIGINS` | *(none)* | Comma-separated allowed `Origin` host patterns for WebSocket connections. Empty disables the origin check (suitable behind an operator-controlled reverse proxy). |

## Logging

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--log-level` | `NORTHWATCH_LOG_LEVEL` | `info` | Minimum log level: `debug`, `info`, `warn` or `error`. |
| `--log-format` | `NORTHWATCH_LOG_FORMAT` | `text` | Structured log output format on stderr: `text` or `json`. |

Logs are emitted as structured, leveled records on stderr. An unrecognized
level or format value fails startup.

## Boolean and duration formats

- Booleans read from environment variables accept `true`/`false`, `1`/`0`
  or `yes`/`no` (case-insensitive). A non-empty value that is none of these
  fails startup rather than silently defaulting.
- Integers read from environment variables must parse as integers; a
  malformed value fails startup.
- Durations use Go's format: `200ms`, `5m`, `1h`, `24h`.

## The `snapshot` subcommand

`northwatch snapshot` captures the live databases to a file for offline replay.
It accepts a subset of the server flags:

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--ovn-nb-addr` | `NORTHWATCH_OVN_NB_ADDR` | *(required)* | Northbound address (comma-separated for failover). |
| `--ovn-sb-addr` | `NORTHWATCH_OVN_SB_ADDR` | *(required)* | Southbound address (comma-separated for failover). |
| `--output`, `-o` | — | `northwatch-snapshot.json` | Output file path. |
| `--monitor-batch-delay` | `NORTHWATCH_MONITOR_BATCH_DELAY` | `200ms` | Same staged-monitor tuning as the server. |
| `--monitor-skip-tables` | `NORTHWATCH_MONITOR_SKIP_TABLES` | *(none)* | Tables to skip during capture. |

It also accepts the six `--ovn-nb-tls-*` / `--ovn-sb-tls-*` flags from
[TLS](#tls), with the same names, env vars and all-or-none rule, so a snapshot
can be captured from `ssl:` databases.

```bash
./bin/northwatch snapshot --ovn-nb-addr tcp:10.0.0.1:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642 -o prod.snapshot.json
```

Serve the result with `./bin/northwatch --snapshot prod.snapshot.json`.

## Examples

```bash
# Minimal single cluster
./bin/northwatch --ovn-nb-addr tcp:10.0.0.1:6641 --ovn-sb-addr tcp:10.0.0.1:6642

# Raft endpoints with failover
./bin/northwatch \
  --ovn-nb-addr tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642

# OpenStack enrichment
./bin/northwatch --ovn-nb-addr tcp:10.0.0.1:6641 --ovn-sb-addr tcp:10.0.0.1:6642 \
  --os-auth-url https://keystone.example.com:5000/v3 \
  --os-username northwatch --os-password secret --os-project-name admin

# Multi-cluster with writes enabled
./bin/northwatch --config-file /etc/northwatch/clusters.json --write-enabled
```
