# Deploy to production

Run Northwatch as a durable service that other people and systems depend on.
This guide covers the decisions that matter once an instance is more than a
throwaway lab: putting it behind an authenticating proxy, keeping load off the
OVN databases, bounding disk growth, and wiring up health checks and metrics.

Northwatch is read-only by default and stays that way unless you opt in, so most
of this guide is about exposure and resource limits rather than the API itself.

## Front it with an authenticating, TLS-terminating proxy

Northwatch has **no built-in authentication or authorization and terminates no
TLS of its own**. The API and UI it serves are plain HTTP, and anyone who can
reach the listen socket can read your entire OVN deployment. A
TLS-terminating, authenticating reverse proxy (nginx, Caddy, an ingress
controller, an identity-aware proxy, …) in front of Northwatch is **mandatory**
for any exposure beyond the loopback interface.

The proxy is responsible for:

- **TLS termination** — Northwatch speaks plain HTTP; the proxy presents the
  certificate and encrypts the client connection.
- **Authentication and authorization** — Northwatch trusts every request it
  receives, so the proxy must decide who may reach it (and, if you enable
  writes, who may mutate OVN).

Built-in authentication and API TLS are tracked work, not a shipped feature —
see [issue #41](https://github.com/B42Labs/northwatch/issues/41). Until it
lands, the proxy is the security boundary.

## Bind the listen socket to loopback

`--listen` (env `NORTHWATCH_LISTEN`) defaults to `:8080`, which binds **every**
interface. In production, bind loopback so the proxy is the only reachable path:

```bash
./bin/northwatch \
  --listen 127.0.0.1:8080 \
  --ovn-nb-addr tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642
```

With the `.deb` package, set `NORTHWATCH_LISTEN=127.0.0.1:8080` in
`/etc/default/northwatch` instead — see [Install on
Debian/Ubuntu](/how-to/install-debian).

## Keep the write API off unless you need it

Write operations are off until you pass `--write-enabled`. Leave them off unless
you have a concrete need. When you do enable them, the same rule applies, harder:
an open API with `--write-enabled` is open OVN mutation. Only enable writes when
the instance is fronted by an authenticating proxy that restricts who can call
`/api/v1/write/*`. See [Enable write operations](/how-to/enable-write-operations)
for the workflow and [Write safety](/explanation/write-safety) for why it is
built the way it is.

## Understand the NB/SB transport

Northwatch connects to the OVN Northbound and Southbound databases over
plaintext `tcp:` today. There is no OVSDB TLS (`ssl:`) for the NB/SB
connections yet — it is tracked in [issue
#41](https://github.com/B42Labs/northwatch/issues/41). Run those connections on
a trusted management network, not across an untrusted one.

(Per-chassis OVS visibility, a separate opt-in feature, does support `ssl:`
management addresses via `--ovs-tls-cert`/`--ovs-tls-key`/`--ovs-tls-ca`; that
is unrelated to the NB/SB transport.)

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
  data has changed. They have **no retention cap yet** — the store keeps every
  snapshot until you delete it. Prune old snapshots manually through the history
  API (`DELETE /api/v1/snapshots/{id}`). An automatic cap is tracked in [issue
  #41](https://github.com/B42Labs/northwatch/issues/41). Budget disk accordingly,
  or lengthen the interval on a large deployment.

Put the history database on persistent storage. The `.deb` preconfigures it under
the systemd `StateDirectory` at `/var/lib/northwatch/history.db`.

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
