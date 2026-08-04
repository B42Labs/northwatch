# Deploy to production

Run Northwatch as a durable service that other people and systems depend on.
This guide covers the decisions that matter once an instance is more than a
throwaway lab: putting it behind an authenticating proxy, keeping load off the
OVN databases, bounding disk growth, and wiring up health checks and metrics.

Northwatch is read-only by default and stays that way unless you opt in, so most
of this guide is about exposure and resource limits rather than the API itself.

## Authenticate every mutating request

Mutating endpoints require a bearer token. Configure one or more with
`--api-tokens` (env `NORTHWATCH_API_TOKENS`), as comma-separated `name=token`
pairs:

```bash
./bin/northwatch \
  --api-tokens "ops=$(openssl rand -hex 32),ci=$(openssl rand -hex 32)" \
  --ovn-nb-addr tcp:10.0.0.1:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642
```

Keep the tokens out of the process table with `--api-tokens-file` (env
`NORTHWATCH_API_TOKENS_FILE`), a JSON object mapping name to token:

```json
{ "ops": "…", "ci": "…" }
```

Clients present the token in the `Authorization` header:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/snapshots \
  -H 'Authorization: Bearer <token>' \
  -d '{"label": "pre-upgrade"}'
```

The name is not a secret — it is the identity recorded as the `actor` on every
write-audit entry, so give each caller its own token. Tokens must be at least 16
characters; the server refuses to start otherwise.

Without tokens, **every mutating endpoint answers 401** — including on loopback.
Read endpoints stay open either way (see the next section).

## Front it with an authenticating, TLS-terminating proxy

The built-in tokens gate mutations. They do **not** protect the read surface:
anyone who can reach the listen socket can still read your entire OVN
deployment, the UI, the WebSocket stream and `/metrics`. A reverse proxy (nginx,
Caddy, an ingress controller, an identity-aware proxy, …) that authenticates
users remains **mandatory** for any exposure beyond loopback.

If the proxy already authenticates every request — including mutating ones —
you can disable the token gate with `--insecure-no-auth`. That is the only way
to run mutating endpoints unauthenticated, it is logged loudly at startup, and
audit entries are then recorded as `anonymous` because there is no credential to
attribute them to.

## Bind the listen socket to loopback

`--listen` (env `NORTHWATCH_LISTEN`) defaults to `127.0.0.1:8080`, so an
unconfigured binary is not reachable from the network. Binding any other address
requires a deliberate decision about authentication: Northwatch **refuses to
start** on a non-loopback address unless you configure `--api-tokens` or pass
`--insecure-no-auth`.

```bash
./bin/northwatch \
  --listen 127.0.0.1:8080 \
  --ovn-nb-addr tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642
```

Container images typically need `NORTHWATCH_LISTEN=0.0.0.0:8080` to be reachable
at all; set tokens (or `--insecure-no-auth`) alongside it, or the container will
exit at startup.

With the `.deb` package, set `NORTHWATCH_LISTEN=127.0.0.1:8080` in
`/etc/default/northwatch` instead — see [Install on
Debian/Ubuntu](/how-to/install-debian).

## Serve HTTPS directly (optional)

Northwatch can terminate TLS itself with `--tls-cert` and `--tls-key` (env
`NORTHWATCH_TLS_CERT` / `NORTHWATCH_TLS_KEY`), which is useful when there is no
proxy to do it — for instance when only the token-gated API is exposed. Both must
be set together, and the minimum protocol version is TLS 1.2. A proxy remains the
better place for certificate lifecycle and user authentication.

## Keep the write API off unless you need it

Write operations are off until you pass `--write-enabled`. Leave them off unless
you have a concrete need. When you do enable them, every mutating write call
needs a token (the `GET` write routes, including the audit log, stay on the open
read surface), and the audit trail records which token made each change. See
[Enable write operations](/how-to/enable-write-operations) for the workflow and
[Write safety](/explanation/write-safety) for why it is built the way it is.

## Encrypt the NB/SB transport

Northwatch dials `ssl:` Northbound and Southbound endpoints when you give it the
client TLS material:

```bash
./bin/northwatch \
  --ovn-nb-addr ssl:10.0.0.1:6641,ssl:10.0.0.2:6641 \
  --ovn-sb-addr ssl:10.0.0.1:6642,ssl:10.0.0.2:6642 \
  --ovn-nb-tls-cert /etc/northwatch/nb.pem \
  --ovn-nb-tls-key  /etc/northwatch/nb-key.pem \
  --ovn-nb-tls-ca   /etc/northwatch/nb-ca.pem \
  --ovn-sb-tls-cert /etc/northwatch/sb.pem \
  --ovn-sb-tls-key  /etc/northwatch/sb-key.pem \
  --ovn-sb-tls-ca   /etc/northwatch/sb-ca.pem
```

Each trio is all-or-none, and an `ssl:` address without TLS material is a startup
error rather than a connection that can never complete a handshake. The same
flags apply to the `northwatch snapshot` subcommand. On a plaintext `tcp:`
deployment, run those connections on a trusted management network.

The TLS material applies to every cluster in a `--config-file`; per-cluster
material is not supported.

(Per-chassis OVS visibility, a separate opt-in feature, has its own
`--ovs-tls-cert`/`--ovs-tls-key`/`--ovs-tls-ca` trio.)

## Size for the deployment

Northwatch holds the entire NB and SB state in memory, so the initial monitor is
the heaviest moment for the OVN databases. On a large Southbound database
(`Logical_Flow` is often more than 90% of all rows) the one-shot initial dump can
spike memory and CPU on `ovsdb-server`. Two flags bound it:

- `--monitor-batch-delay` (default `200ms`) stages the monitor as one request
  per table instead of one giant `monitor_all`.
- `--monitor-skip-tables` never monitors the listed tables at all — the real
  lever for the biggest tables (e.g. `--monitor-skip-tables Logical_Flow`).

For the reasoning and recommended settings per deployment size, see [Large
deployments](/explanation/large-deployments) and [Tune the initial
load](/how-to/tune-the-initial-load). Where possible, also point Northwatch at an
OVSDB relay or a standby/follower rather than the Raft leader.

## Bound SQLite growth

A running server records into an embedded SQLite history database
(`--history-db-path`). Two kinds of data grow it, and they are bounded
differently:

- **Events** (the OVSDB change stream) are pruned by age with `--event-retention`
  (default `24h`) and can additionally be capped by count with
  `--event-max-count` (default `0`, meaning unlimited). Set a count cap on a busy
  deployment so a churn spike cannot grow the database without bound.
- **Auto-snapshots** run every `--snapshot-interval` (default `5m`) whenever the
  data has changed. Northwatch keeps the newest `--snapshot-max-count` of them
  (default `500`, `0` disables pruning) and deletes the rest. Snapshots you took
  deliberately — manual, labeled or imported ones — are **never** pruned; remove
  those through the history API (`DELETE /api/v1/snapshots/{id}`). Lower the cap
  or lengthen the interval on a large deployment, where each snapshot is a full
  copy of the database.

Put the history database on persistent storage. The `.deb` creates
`/var/lib/northwatch` and preconfigures the database at
`/var/lib/northwatch/history.db`.

## Wire up health checks

Northwatch exposes two endpoints for orchestrators and load balancers:

| Endpoint | Meaning | Use it for |
|---|---|---|
| `GET /healthz` | Liveness. Always returns `200` while the process serves HTTP. | Restart policy — is the process alive? |
| `GET /readyz` | Readiness. Returns `200` only when both the NB and SB clients are connected and the live monitors are not suspended; otherwise `503`. | Traffic gating — should this instance receive requests? |

Point your restart supervision at `/healthz` and your traffic/load-balancer
readiness gate at `/readyz`. Note that loading a snapshot session suspends the
live monitors, so `/readyz` deliberately flips to `503` for the duration (live
data is stale while a snapshot is loaded) even though `/healthz` stays `200`.

```bash
curl -s http://localhost:8080/healthz   # {"status":"ok"} — always 200
curl -s http://localhost:8080/readyz    # {"status":"ready"} 200, or {"status":"not ready"} 503
```

## Scrape metrics

Northwatch serves Prometheus metrics at `/metrics` (always on), including OVSDB
connection state, per-table row counts, config-propagation lag, and its own HTTP
server metrics. Wire it into your monitoring and alert on the connection gauges
and propagation lag. See [Scrape Prometheus
metrics](/how-to/scrape-prometheus-metrics) for scrape configuration and
[Prometheus metrics](/reference/metrics) for the full metric set.

## Run it under systemd

The release `.deb` installs Northwatch as a hardened systemd service that runs as
a dedicated unprivileged `northwatch` user, restarts on failure, and logs to
journald. Prefer it over a hand-rolled process supervisor. See [Install on
Debian/Ubuntu](/how-to/install-debian) for the package, its files, and the
service lifecycle.

## Related

- [Install on Debian/Ubuntu](/how-to/install-debian)
- [Tune the initial load](/how-to/tune-the-initial-load)
- [Large deployments](/explanation/large-deployments)
- [Write safety](/explanation/write-safety)
- [The capability model](/explanation/capability-model)
