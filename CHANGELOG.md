# Changelog

All notable changes to Northwatch are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `--log-level` (`NORTHWATCH_LOG_LEVEL`, `debug`/`info`/`warn`/`error`) and
  `--log-format` (`NORTHWATCH_LOG_FORMAT`, `text`/`json`) flags. Logs are now
  emitted as structured, leveled records on stderr.
- `--version` flag: prints the build version and exits. Release builds
  inject the tag via `-ldflags "-X main.version=<tag>"`; other builds
  report `dev`.
- Fleet-wide OVS health on the OVS Visibility overview. A new
  `GET /api/v1/ovs/health` aggregates the per-chassis monitored caches into
  fleet totals — bridges, ports and interfaces — plus counts of interfaces
  that are down (`link_state=down`) or erroring (non-zero `rx/tx` errors or
  drops), with a per-chassis breakdown. Aggregation runs off the caches (no
  new connections) and handles partial outages: an unreachable chassis is
  excluded from every total (never counted as healthy) but still listed with
  `connected=false`. The overview tiles now show these totals and a down/error
  indicator, and the chassis picker flags a chassis with down or erroring
  interfaces beyond its connection dot. See
  [HTTP API](https://b42labs.github.io/northwatch/reference/api).
- Opt-in, read-only per-chassis Open_vSwitch (vswitchd) visibility. When the
  server is started with `--ovs-mgmt-addr-file` (a JSON map of chassis
  system-id to OVSDB management address), Northwatch opens one monitored
  libovsdb connection per chassis and exposes the live OVS state the
  Southbound DB cannot provide: `GET /api/v1/ovs` for fleet connection status
  and `GET /api/v1/ovs/{chassis}/{table}[/{uuid}]` for the `interface`
  (`statistics`, `link_state`, `error`, …), `bridge`, `port`, `open-vswitch`,
  `manager` and `controller` (`is_connected`) tables. `{chassis}` is the
  system-id, joining directly to the SB `Chassis.name` /
  `external_ids:system-id`. The connection pool tolerates partial outages —
  one unreachable chassis keeps retrying in isolation and never breaks the
  others or the NB/SB views (an unreachable chassis returns `503`, an unknown
  one `404`). `ssl:` endpoints are supported via `--ovs-tls-cert`/`-key`/`-ca`.
  Disabled by default and wired to the default cluster; advertised by the new
  `ovs` capability. See
  [HTTP API](https://b42labs.github.io/northwatch/reference/api) and
  [CLI flags](https://b42labs.github.io/northwatch/reference/cli).
- The per-chassis OVS view now routes all 19 vswitchd tables the monitored
  `Open_vSwitch` cache already holds (up from 6) and lists them in the table
  picker — the monitoring/export-config tables (`ipfix`, `sflow`, `netflow`,
  `mirror`, `qos`, `queue`), the conntrack tables (`ct-zone`,
  `ct-timeout-policy`, `datapath`) and the rest (`flow-table`,
  `flow-sample-collector-set`, `ssl`, `autoattach`). No new connections; the
  `ssl` row omits `private_key` so SSL key material is never served (the
  public `certificate`/`ca_cert` stay).
- The per-chassis OVS list and detail views now resolve UUID reference
  columns (`Bridge.ports`, `Port.interfaces`, `Bridge.controller`,
  `Bridge.mirrors`, and similar) to the target row's `name` (or `target`
  for `controller`/`manager`) and link each to that row's detail view, so a
  Bridge → its Ports → their Interfaces can be walked without switching
  tables and matching UUIDs by hand. Resolution stays within the selected
  chassis cache (no cross-chassis joins); references whose target row is
  unfetched or nameless still link but show the short UUID.
- Live OVS state is now correlated with OVN intent — the *correlate*
  differentiator. `GET /api/v1/ovs/{chassis}/interface/{uuid}/correlation`
  resolves an interface's `external_ids:iface-id` to the Southbound
  `Port_Binding` whose `logical_port` matches, surfacing the bound logical
  port, its `up` state, `datapath` and bound `chassis` next to the live
  interface, and flags `drift` where the Southbound reports the port up but
  the interface is down or erroring. The OVS interface detail view renders an
  **OVN correlation** section from this endpoint; it degrades gracefully when
  the `iface-id` or the Southbound mapping is absent. The OVS Visibility and
  Chassis Inventory pages now link each other by system-id (`?chassis=`). Reads
  only the Southbound cache (no NB dependency, no new OVSDB connections) and is
  wired to the default cluster. See
  [HTTP API](https://b42labs.github.io/northwatch/reference/api).
- Aggregated chassis inventory built entirely from the existing Southbound
  cache (no new OVSDB connections): `GET /api/v1/sb/chassis-inventory` and
  `/{name}`. Each entry joins `Chassis`, `Encap`, `Chassis_Private`,
  `SB_Global` and `Port_Binding` into one chassis-centric view — system-id
  (the Phase-2 OVS join key), tunnel endpoints, bridge mappings, a computed
  liveness (`in_sync` and `alive` from `nb_cfg` sync; `nb_cfg_timestamp` age
  surfaced as informational `age_ms` and a `stale` flag for a lagging chassis)
  and a per-chassis bound-port summary. A chassis is `alive` when present and
  in-sync, so a steady-state cluster with no config churn stays healthy rather
  than reporting every chassis down once `nb_cfg_timestamp` freezes. The
  staleness threshold for the `stale` flag is configurable via
  `--chassis-stale-threshold` / `NORTHWATCH_CHASSIS_STALE_THRESHOLD`
  (default `60s`).
- Dashboard **Chassis Inventory** view (Monitoring section, `/chassis-inventory`)
  surfacing the aggregated endpoint: fleet stat tiles (alive / in-sync / down /
  bound ports), a liveness filter, and a per-chassis table with expandable rows
  that lazy-load the detail (`other_config` and bound logical ports).
- Debian `.deb` package, built and cosign-signed on every tagged release
  (nfpm + a hardened systemd unit). It installs the static binary to
  `/usr/bin/northwatch`, runs it as a dedicated unprivileged `northwatch`
  system user with the SQLite history DB under the systemd `StateDirectory`
  (`/var/lib/northwatch`), and ships `/etc/default/northwatch` as a conffile so
  operator edits survive upgrades. `remove` keeps the history DB; `purge` drops
  it (retaining the system user). Build one locally with `make deb`. See
  [Install on Debian/Ubuntu](https://b42labs.github.io/northwatch/how-to/install-debian).
- OpenStack enrichment can now verify the Keystone/Nova API against a private CA
  via `--os-cacert` / `OS_CACERT` (maps to the clouds.yaml `cacert` field). The
  CA is scoped to the OpenStack HTTP client, leaving the rest of the process on
  the system trust store.
- `make testbed`: runs Northwatch against the OSISM testbed control plane
  (`192.168.16.10/11/12`, NB/SB failover across all three) with OpenStack name
  resolution. Credentials mirror `clouds.yaml` and the API is verified against
  `testbed.pem`. Connection details and credentials are overridable
  (`TESTBED_CP*`, `TESTBED_NB`/`TESTBED_SB`, `OS_*`).
- Local OVN lab under `lab/`: a containerlab topology (1 `central` running
  ovn-northd + NB/SB, 3 `chassis` running OVS userspace datapath +
  ovn-controller) plus `make lab-*` targets (`lab-up`, `lab-down`, `lab-seed`,
  `lab-sim`, `lab-clean`, `lab-nbctl`/`lab-sbctl`, `lab-multi-up`) to spin up a
  complete test environment for the dashboard. A Docker Compose variant
  (`lab/docker-compose.yml`, `make lab-compose-up`/`lab-compose-down`) runs the
  same topology without containerlab — handy on macOS / Docker Desktop.
- `ovnsim` load generator (`cmd/ovnsim`, `internal/ovnsim`): `seed` creates an
  idempotent baseline across every major NB table, `run` continuously mutates
  the topology (create/delete switches, routers, ports, NAT, ACLs, LB VIPs;
  optional real port binding/migration onto chassis via `--bind-ports`),
  `bind`/`unbind` (and `make lab-bind`/`lab-unbind`) bind all seeded VIFs onto
  chassis so they bind in SB (clearing the "VIF not bound to any chassis"
  alerts), and `clean` removes everything it created. All objects are tagged so
  it only ever touches its own rows.
- Opt-in OVS kernel-datapath mode for the lab chassis (`DATAPATH_TYPE=system`),
  via `lab/docker-compose.kernel.yml` / `make lab-compose-up KERNEL=1`. On a
  Linux host with the `openvswitch` kernel module it makes BFD between chassis
  converge, so multi-member `HA_Chassis_Group` gateways actually bind (the
  gateway "no-owner" state clears). The default stays the portable userspace
  (netdev) datapath.
- `ovnsim` now seeds `HA_Chassis_Group`s (even-numbered routers reference one,
  odd-numbered routers use per-port `Gateway_Chassis`) and simulates gateway
  failover at runtime by swapping HA group member priorities and adding/removing
  members.
- `ovnsim run --bind-ports` now keeps the lab healthy: it binds every existing
  unbound VIF on startup and binds each port it creates (via `addPort` /
  `createSwitch`), and the alert-generating random "unbind" action was removed
  (explicit `ovnsim unbind` / `make lab-unbind` still available). This stops the
  "VIF not bound to any chassis" alerts from accumulating while the simulator
  runs.
- Configurable WebSocket Origin allowlist via `--ws-allowed-origins` /
  `NORTHWATCH_WS_ALLOWED_ORIGINS`. When unset, origin checking is disabled
  (single-tenant deployment default).
- Tests for impact, cluster proxy, lb_topology, nat_topology, flow_tables,
  and uuid handlers.
- Doc comments on previously undocumented exported symbols in `internal/ovsdb`,
  `internal/search`, and `internal/api`.

### Fixed
- Write operations touching non-scalar columns (`external_ids`, `options`,
  set/reference fields) now round-trip correctly: row construction is routed
  through the typed hydrator plus libovsdb's mapper, so map/set/UUID fields are
  wire-encoded as RFC 7047 `["map",…]`/`["set",…]`/`["uuid",…]` instead of being
  marshaled as plain JSON and rejected or corrupted at apply.
- Apply now re-validates the plan and asserts each target row still holds its
  pre-apply values with OVSDB `wait` operations, so an update to a since-changed
  or since-deleted row aborts with a conflict instead of silently no-op-ing.
- The plan cache returns a copy, ending a data race between a plan being applied
  and the same plan being serialized by `GET /api/v1/write/plans/{id}`.
- Write reference validation is derived from the NB schema (all writable tables,
  not three), and cache-list failures are reported as server errors rather than
  treated as "does not exist".
- Audit-store insert failures are logged instead of silently dropped.
- Loading a snapshot session no longer corrupts monitoring: the history
  collector, alert engine, and flowdiff collector are paused while the live
  monitors are suspended, so there are no empty "auto" snapshots, no false
  `flow_count_anomaly` (or webhook) on the 100% cache drop, and no
  re-population event flood on resume.
- Gateway and fleet health are correct on OVN ≥ 20.06: chassis liveness is
  derived from `Chassis_Private.nb_cfg` vs `SB_Global.nb_cfg` (a shared helper
  with the chassis inventory) instead of the deprecated `Chassis.nb_cfg`, which
  reported every chassis stale and produced fleet-wide false
  `no-viable-candidate` errors. The `stale_chassis_config` alert uses the same
  signal.
- Silenced alerts no longer fire webhooks or WebSocket events — silences are
  now consulted before notifying, for both fire and resolve.
- Alert rules distinguish a read failure from "no alerts": a rule that errors
  (e.g. during an OVSDB reconnect) no longer resolves and re-fires every active
  alert. `flow_count_anomaly` ignores a zero-count read (a purged cache).
- `uuid[:8]` display truncation is rune-safe and no longer panics on short
  identifiers (alert HA-failover messages, topology labels, SVG export).
- Multi-cluster UI no longer mixes clusters: the MAC-table, NAT and HA-failover
  pages route reads through the cluster-scoped API client, and the WebSocket
  connects to the active cluster's endpoint (reconnecting on cluster change)
  instead of always the default cluster.
- The cluster switcher surfaces a "sources unavailable" indicator when the
  cluster list fails to load instead of silently disappearing.
- The startup connect timeout scales with `--monitor-batch-delay` × table
  count, so a large staged-load delay documented for huge deployments no longer
  deterministically fails startup. The `snapshot` subcommand rejects a
  malformed `NORTHWATCH_MONITOR_BATCH_DELAY` instead of silently ignoring it.
- Both OVSDB clients are closed on any `Connect` error path, and a `Connect`
  called per snapshot load no longer leaks reconnect state.
- Live events in the Events view now show the server-side event time
  (`ts`) instead of the client receive time — the view read a
  `wsEvent.timestamp` field that does not exist on the WebSocket payload.
- The generated OpenAPI spec described the write rollback operation as
  "Rollback to a snapshot (not yet implemented)" with a `501` response, but the
  feature is implemented and returns `200` with a reversing preview plan. The
  summary and response are corrected.
- `ovnsim` now models gateways the way ovn-northd expects: each router has one
  distributed gateway port attached to a dedicated external switch with a
  `localnet` port (carrying an HA_Chassis_Group on even routers, a
  Gateway_Chassis on odd ones); tenant ports are plain patch ports. ovn-northd
  only realizes the `chassisredirect` ports — and the SB HA_Chassis_Group — that
  the gateway / HA failover view is built from when the distributed gateway port
  connects to an externally-connected switch, so the previous setup (every tenant
  port a gateway port, none localnet-connected) left that view empty. Added
  `make lab-reseed` (clean + seed + bind) to force a fresh baseline, since `seed`
  is idempotent by name and skips existing objects.
- Port `type_consistency` diagnostic no longer warns on router ports. A "router"
  Logical_Switch_Port is realized in SB as a `patch` Port_Binding (or
  `l3gateway`/`chassisredirect` for distributed gateway ports), so the previous
  exact string comparison flagged every router port in every deployment.

### Changed
- Malformed boolean/integer environment variables (`NORTHWATCH_WRITE_ENABLED`,
  `NORTHWATCH_KUBE_ENRICHMENT`, `NORTHWATCH_WRITE_RATE_LIMIT`,
  `NORTHWATCH_EVENT_MAX_COUNT`) now fail startup instead of silently
  defaulting. Booleans additionally accept `false`/`0`/`no` and the
  case-insensitive `strconv.ParseBool` forms, so `NORTHWATCH_WRITE_ENABLED=True`
  correctly means true (previously it silently meant false).
- Write **rollback** is scoped to restoring changed fields on rows that still
  exist. Rows deleted since the snapshot are reported in a new plan `warnings`
  field rather than recreated (recreation would assign new UUIDs and dangle
  every reference), and rows created after the snapshot are left untouched.
- Write endpoints return more precise status codes: `429` for rate-limit
  rejections (dry-run is now rate-limited too), `409` for a stale-state
  conflict at apply, `400` for input errors, and `500` for infrastructure
  failures — instead of mapping every engine error to `400`.
- `/readyz` returns `503` while a snapshot session is loaded (the live monitors
  are suspended and the caches purged), and responses served from a
  not-ready/suspended cluster carry an `X-Northwatch-Stale: true` header so
  clients can flag stale data.
- Loaded snapshot sessions are capped (default 4, `409` when exceeded), the slow
  load work runs outside the manager lock, and a session's backing servers are
  torn down after a short drain grace so in-flight requests finish.
- Snapshot **load** tolerates dangling references: a capture from a churning
  database whose rows reference since-deleted rows loads with the dangling
  references pruned instead of failing wholesale.
- Fleet OVS health is delta-based: an interface is flagged erroring or dropping
  only when its cumulative `rx/tx` counters increase between reads (not on any
  non-zero lifetime counter), so a single historical error no longer trends the
  tile permanently red. Drops are reported separately via a new
  `drop_interfaces` field, and the fleet tile counters reset on process restart.
- `--chassis-stale-threshold` now also drives gateway HA-member liveness and the
  `stale_chassis_config` alert (not only the chassis inventory).
- CI is now a trustworthy gate: the test job runs the race detector,
  `svelte-check` runs on the frontend, gofmt is enforced via
  golangci-lint, tool versions (golangci-lint, govulncheck) are pinned,
  a scheduled workflow runs `govulncheck` daily against `main`, and
  Dependabot keeps Go modules, npm packages, and GitHub Actions updated.
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
