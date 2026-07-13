# HTTP API

All routes are under `/api/v1` and return JSON; health and metrics endpoints sit
at the root. The API is **read-only by default** — mutation requires
`--write-enabled` (see [Enable write operations](/how-to/enable-write-operations)).

::: tip The OpenAPI spec is authoritative
This page lists the route groups. For exact query parameters and response
schemas, use the spec the server generates: Swagger UI at `/api/v1/docs`, raw
spec at `/api/v1/openapi.json`. See [Explore the API](/how-to/explore-the-api).
:::

## Authentication

Every **mutating** request (`POST`, `PUT`, `DELETE` under `/api/`) must present a
bearer token configured with `--api-tokens`:

```
Authorization: Bearer <token>
```

Without a valid token the request is rejected with `401` and a
`WWW-Authenticate: Bearer realm="northwatch"` header, before the handler runs.
The token is accepted from the header only — never a query parameter. With no
tokens configured, every mutating endpoint answers `401`.

Read endpoints (`GET`) need no token; protect them with a reverse proxy. See
[Deploy to production](/how-to/deploy-production).

## Response shape

- List endpoints return a JSON **array** of objects. Each object's keys are the
  OVSDB column names of that table (from the model's `ovsdb` struct tags).
- Detail endpoints (`.../{uuid}`) return a single object, or `404` with
  `{"error": "not found"}`.
- Errors return the matching HTTP status with `{"error": "<message>"}`.
- `5xx` responses always carry the generic body `{"error": "internal server
  error"}`. The cause is logged server-side with the request that triggered it —
  it is deliberately not sent to the client.
- Responses served from a cluster whose OVSDB caches are not currently
  authoritative — the client is reconnecting, or a snapshot session has suspended
  the live monitors — carry an `X-Northwatch-Stale: true` header. `/readyz`
  returns `503` in the suspended case.

## Limits

The API bounds what a single request can cost, so a client cannot exhaust the
process:

| Limit | Value | Applies to |
|---|---|---|
| Page size | `limit` / `offset` query params, default and maximum **5000** rows | Every table-list endpoint, including `/api/v1/sb/logical-flows` |
| Request body | **1 MiB** | Every endpoint that accepts a body |
| Request body (import) | **100 MiB** | `POST /api/v1/snapshots/import` |
| Search matches | **100** per table, **500** total | `GET /api/v1/search` |

List responses carry `X-Total-Count` (the unpaginated total) and, when rows were
dropped, `X-Truncated: true`. Rows are ordered by UUID, so `offset` steps through
a stable sequence. An oversized body is rejected with `413`; an invalid `limit`
or `offset` with `400`.

`GET /api/v1/search` returns a `truncated` field reporting whether the match caps
dropped results — refine the query when it is set.

`GET /api/v1/debug/trace` retains the trace for later export only when called
with `?store=true`, and returns its `id` only then.

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
against `SB_Global.nb_cfg`, and `alive` is true when the chassis is present and
in-sync. `alive` deliberately ignores `nb_cfg_timestamp` age — that timestamp
only advances on a new `nb_cfg` generation, so on a steady-state cluster it
freezes and an age check would report every healthy chassis down. Instead,
`age_ms` is surfaced informationally and `stale` flags an *out-of-sync* chassis
that has lagged beyond `--chassis-stale-threshold` (lagging/stuck, distinct from
down). A chassis with no `Chassis_Private` row is reported `in_sync=false,
alive=false`. `name` (= the OVS `external_ids:system-id`) is the join key to the
real Open_vSwitch instance.

These are the *aggregated* views; the raw `chassis`, `encaps`, `port-bindings`
and `chassis-private` tables remain available under [Southbound tables](#southbound-tables).

## OVS (per-chassis Open_vSwitch)

Live, read-only state from each chassis's local Open_vSwitch (vswitchd) OVSDB —
the *reality* the Southbound DB only describes as *intent* (is the interface
actually up? rx/tx stats? errors? is `ovn-controller` attached to `br-int`?).
This surface is **opt-in**: it appears only when the server is started with
`--ovs-mgmt-addr-file` (see [CLI flags](/reference/cli)), and the `ovs`
[capability](/reference/capabilities) advertises it.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/ovs` | Fleet status: one entry per configured chassis with its `system_id`, management `addr` and whether Northwatch is `connected`. |
| GET | `/api/v1/ovs/health` | Aggregated fleet-wide health: bridge/port/interface totals and down/erroring interface counts across connected chassis, with a per-chassis breakdown. |
| GET | `/api/v1/ovs/{chassis}/{table}` | List rows of one OVS table on one chassis. |
| GET | `/api/v1/ovs/{chassis}/{table}/{uuid}` | One row by UUID. |
| GET | `/api/v1/ovs/{chassis}/interface/{uuid}/correlation` | Correlate a live interface with its Southbound `Port_Binding`. |

`{chassis}` is the **system-id** — the same `name` as in the [chassis
inventory](#chassis-inventory) (`external_ids:system-id` on the OVS instance) —
so it joins directly to the SB view. `{table}` is one of the whitelisted
vswitchd tables (the OVSDB table name lowercased with `_` rendered as `-`):

| `{table}` | What the OVS instance holds |
|---|---|
| `interface` | `statistics` (rx/tx packets/bytes/errors/dropped), `link_state`, `admin_state`, `link_speed`, `mtu`, `ofport`, `mac_in_use`, `error`, driver `status`. |
| `bridge` | The real bridges (`br-int`, `br-ex`, provider bridges), `datapath_type`, `datapath_id`, physical ports. |
| `port` | Bridge ports and their bonding/VLAN config. |
| `open-vswitch` | `ovs_version`, DPDK state, available `iface_types`/`datapath_types`. |
| `manager` | `is_connected` — whether the OVSDB manager connection is up. |
| `controller` | `is_connected` — whether `ovn-controller` is attached to `br-int`. |
| `ipfix` | IPFIX flow-export config — collector `targets`, sampling, cache timeouts. |
| `sflow` | sFlow agent config — collector `targets`, sampling and polling. |
| `netflow` | NetFlow collector config — `targets`, active timeout, engine IDs. |
| `mirror` | Port-mirror (SPAN/RSPAN) config — selected source/output ports. |
| `qos` | QoS policies attached to ports — `type`, queues, `other_config`. |
| `queue` | Per-queue config referenced by QoS — DSCP, min/max rate. |
| `ct-zone` | Conntrack zone → timeout-policy bindings. |
| `ct-timeout-policy` | Conntrack timeout policies by protocol state. |
| `datapath` | Datapath instances — `datapath_version`, capabilities, conntrack zones. |
| `flow-table` | OpenFlow table config — name, flow limits, eviction policy. |
| `flow-sample-collector-set` | IPFIX flow-sample collector sets bound to a bridge. |
| `ssl` | SSL config — `certificate`, `ca_cert`, `bootstrap_ca_cert`. `private_key` is **omitted** so key material is never served. |
| `autoattach` | IEEE 802.1AB Auto-Attach mappings. |

Reachability semantics:

- An unknown chassis (not in the mapping) or an unknown table returns **404**.
- A registered chassis that is currently **unreachable** returns **503**
  (`chassis unreachable`) — distinct from 404, so an outage is observable. One
  dead chassis never affects the others or the NB/SB views.

**Fleet health.** `GET /api/v1/ovs/health` turns the per-chassis connection
counts into a real health summary. It sums `bridges`, `ports` and `interfaces`
across all connected chassis and counts interfaces that are **down**
(`link_state=down`), **erroring** (`rx_errors`/`tx_errors` that increased since
the previous read) or **dropping** (`rx_dropped`/`tx_dropped` that increased) as
`down_interfaces`, `error_interfaces` and `drop_interfaces`, alongside `chassis`,
`connected` and `unreachable` counts. Erroring and dropping are **delta-based**:
the cumulative counters are compared between reads, so a single historical error
does not flag an interface forever (the counters reset on process restart).
`members` is the per-chassis breakdown (`system_id`, `connected`, and the same
counts per chassis). Aggregation runs off the monitored caches — no new
connections — and handles partial outages: an **unreachable** chassis is
**excluded from every total** (never counted as healthy) but still listed in
`members` with `connected=false` and zero counts, so a green aggregate can
never hide a problem chassis.

**OVN correlation.** `GET /api/v1/ovs/{chassis}/interface/{uuid}/correlation`
ties live OVS state to OVN intent — the Northwatch *correlate* differentiator.
It resolves the interface's `external_ids:iface-id` to the Southbound
`Port_Binding` whose `logical_port` matches, surfacing the bound logical port,
its `up` state, `datapath` and bound `chassis` (with `bound_here` set when the
port is bound on this same chassis). `drift` flags where the Southbound reports
the port up but the live interface is down or erroring. Correlation degrades
gracefully: `bound` is `false` with no `binding` when the interface carries no
`iface-id` or no matching `Port_Binding` exists. It reads only the Southbound
cache (no NB dependency) and, like the table routes, returns **404** for an
unknown chassis or interface UUID and **503** when the chassis is unreachable.

**Operator setup.** `ovsdb-server` listens only on a local Unix socket by
default; remote access must be enabled per chassis, e.g.
`ovs-vsctl set-manager ptcp:6640` (or `pssl:6640` for TLS). The
`--ovs-mgmt-addr-file` mapping then points Northwatch at each chassis's
management address.

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
| GET | `/api/v1/debug/port-diagnostics` · `/{uuid}` | Unbound / mismatched ports. The list form accepts `?severity=healthy\|warning\|error` and `?limit=<n>` to filter and cap the returned `ports` (the summary counts stay totals); an invalid value returns `400`. |
| GET | `/api/v1/debug/nexthop-mac` | Next-hop MAC resolution. |
| GET | `/api/v1/debug/acl-audit` | Shadowed / conflicting ACLs, compared only within the same attachment scope (owning switch or port group). The response sets `truncated: true` when the finding cap is reached. |
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
