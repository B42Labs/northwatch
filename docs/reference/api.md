# HTTP API

All routes are under `/api/v1` and return JSON; health and metrics endpoints sit
at the root. The API is **read-only by default** — mutation requires
`--write-enabled` (see [Enable write operations](/how-to/enable-write-operations)).

::: tip The OpenAPI spec is authoritative
This page lists the route groups. For exact query parameters and response
schemas, use the spec the server generates: Swagger UI at `/api/v1/docs`, raw
spec at `/api/v1/openapi.json`. See [Explore the API](/how-to/explore-the-api).
:::

## Response shape

- List endpoints return a JSON **array** of objects. Each object's keys are the
  OVSDB column names of that table (from the model's `ovsdb` struct tags).
- Detail endpoints (`.../{uuid}`) return a single object, or `404` with
  `{"error": "not found"}`.
- Errors return the matching HTTP status with `{"error": "<message>"}`.

## System & health

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | Liveness probe. |
| GET | `/readyz` | Readiness probe (ready after initial sync). |
| GET | `/metrics` | Prometheus metrics. |
| GET | `/api/v1/capabilities` | Active capabilities and mode. |
| GET | `/api/v1/clusters` | List configured clusters. |
| GET | `/api/v1/docs` | Swagger UI. |
| GET | `/api/v1/openapi.json` | OpenAPI 3.1 spec. |

## Northbound tables

Each table exposes `GET /api/v1/nb/<table>` (list) and
`GET /api/v1/nb/<table>/{uuid}` (detail):

`logical-switches`, `logical-switch-ports`, `logical-routers`,
`logical-router-ports`, `acls`, `nats`, `address-sets`, `port-groups`,
`load-balancers`, `load-balancer-groups`, `logical-router-policies`,
`logical-router-static-routes`, `dhcp-options`, `nb-global`, `connections`,
`dns`, `gateway-chassis`, `ha-chassis-groups`, `ha-chassis`, `meters`, `qos`,
`bfd`, `copp`, `mirrors`, `forwarding-groups`, `static-mac-bindings`,
`load-balancer-health-checks`.

## Southbound tables

Same `list` + `/{uuid}` pattern under `/api/v1/sb/<table>`:

`chassis`, `port-bindings`, `datapath-bindings`, `logical-flows`, `encaps`,
`mac-bindings`, `fdb`, `multicast-groups`, `address-sets`, `port-groups`,
`load-balancers`, `dns`, `sb-global`, `connections`, `gateway-chassis`,
`ha-chassis-groups`, `ha-chassis`, `ip-multicast`, `igmp-groups`,
`service-monitors`, `bfd`, `meters`, `mirrors`, `chassis-private`,
`controller-events`, `static-mac-bindings`, `logical-dp-groups`, `rbac-roles`,
`rbac-permissions`.

## Chassis inventory

An aggregated, chassis-centric view of the hypervisor/gateway fleet, computed
entirely from the existing Southbound cache (no extra connections). Each entry
joins `Chassis` + `Encap` + `Chassis_Private` + `SB_Global` + `Port_Binding`:

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/sb/chassis-inventory` | One entry per chassis: system-id, tunnel endpoints, bridge mappings, liveness, bound-port summary. |
| GET | `/api/v1/sb/chassis-inventory/{name}` | Detail for one chassis (by `name` / system-id), including the config copies and the list of bound logical ports. |

Liveness is computed, not stored: `in_sync` compares `Chassis_Private.nb_cfg`
against `SB_Global.nb_cfg`, and `alive` checks that `nb_cfg_timestamp` is fresh
within `--chassis-stale-threshold`. A chassis with no `Chassis_Private` row is
reported `in_sync=false, alive=false`. `name` (= the OVS `external_ids:system-id`)
is the join key to the real Open_vSwitch instance.

These are the *aggregated* views; the raw `chassis`, `encaps`, `port-bindings`
and `chassis-private` tables remain available under [Southbound tables](#southbound-tables).

## Correlated views

Northbound entities joined to their Southbound counterpart and enrichment context:

| Method | Path |
|---|---|
| GET | `/api/v1/correlated/logical-switches` · `/{uuid}` |
| GET | `/api/v1/correlated/logical-switch-ports/{uuid}` |
| GET | `/api/v1/correlated/logical-routers` · `/{uuid}` |
| GET | `/api/v1/correlated/logical-router-ports/{uuid}` |
| GET | `/api/v1/correlated/port-bindings/{uuid}` |
| GET | `/api/v1/correlated/chassis` · `/{uuid}` |

## Search

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/search?q=<query>` | Omnisearch across NB, SB and enrichment. |

## Topology

| Method | Path |
|---|---|
| GET | `/api/v1/topology` |
| GET | `/api/v1/topology/gateway` |
| GET | `/api/v1/topology/nat` |
| GET | `/api/v1/topology/load-balancers` |
| GET | `/api/v1/flows` |

## Debug (the `debug` capability)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/debug/trace` | Packet path trace. |
| GET | `/api/v1/debug/traces` | Recent traces. |
| GET | `/api/v1/debug/connectivity` | Path analysis between two ports. |
| GET | `/api/v1/debug/port-diagnostics` · `/{uuid}` | Unbound / mismatched ports. |
| GET | `/api/v1/debug/nexthop-mac` | Next-hop MAC resolution. |
| GET | `/api/v1/debug/acl-audit` | Shadowed / conflicting ACLs. |
| GET | `/api/v1/debug/stale-entries` | Stale MAC/FDB and orphaned bindings. |
| GET | `/api/v1/debug/flow-diff` | Recent logical-flow changes. |
| GET | `/api/v1/impact/{db}/{table}/{uuid}` | Impact analysis for an entity. |

## Telemetry

| Method | Path |
|---|---|
| GET | `/api/v1/telemetry/summary` |
| GET | `/api/v1/telemetry/cluster` |
| GET | `/api/v1/telemetry/flows` |
| GET | `/api/v1/telemetry/raft-health` |
| GET | `/api/v1/telemetry/propagation` · `/heatmap` · `/timeline` |

## Alerts

| Method | Path |
|---|---|
| GET | `/api/v1/alerts` |
| GET | `/api/v1/alerts/rules` |
| PUT | `/api/v1/alerts/rules/{name}` |
| GET / POST | `/api/v1/alerts/silences` |
| DELETE | `/api/v1/alerts/silences/{id}` |

## History, snapshots & events

| Method | Path |
|---|---|
| GET | `/api/v1/events` |
| GET / POST | `/api/v1/snapshots` |
| GET | `/api/v1/snapshots/{id}` · `/rows` · `/export` |
| GET | `/api/v1/snapshots/diff` |
| DELETE | `/api/v1/snapshots/{id}` |
| POST | `/api/v1/snapshots/import` |
| POST | `/api/v1/snapshots/{id}/load` · `/unload` |

## Export

| Method | Path |
|---|---|
| GET | `/api/v1/export/topology` |
| GET | `/api/v1/export/trace/{id}` |

## WebSocket

| Path | Purpose |
|---|---|
| `/api/v1/ws` | Real-time stream of OVSDB change events. |

The `--ws-allowed-origins` flag controls the `Origin` allowlist.

## Write (only with `--write-enabled`)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/write/schema` | What can be changed. |
| POST | `/api/v1/write/preview` · `/dry-run` | Predict an effect without applying. |
| GET | `/api/v1/write/plans/{id}` | Inspect a prepared plan. |
| POST | `/api/v1/write/plans/{id}/apply` | Apply a plan. |
| DELETE | `/api/v1/write/plans/{id}` | Cancel a plan. |
| POST | `/api/v1/write/failover` · `/evacuate` · `/rollback` · `/restore` | Operational actions. |
| GET | `/api/v1/write/audit` · `/{id}` | Audit log. |

## Per-cluster routes

In a multi-cluster setup every cluster is reachable under a prefix, mirroring the
browsing routes above:

```
/api/v1/clusters/{name}/...
```

The default cluster is served both at the top level and under its prefix. See
[Monitor multiple clusters](/how-to/monitor-multiple-clusters).
