# Northwatch

Real-time Analyzer, Visualizer, Debugger, Configurator, Telemetry & Monitoring for OVN.

## Overview

Northwatch connects directly to the OVN Northbound and Southbound OVSDB databases via TCP and provides a unified interface for operating, debugging and monitoring OVN deployments. It supports pluggable enrichment providers -- OpenStack is the primary integration, resolving UUIDs to human-readable names for networks, ports, instances, routers, etc. Additional providers (Kubernetes, static mappings) can be added via the enrichment interface.

Northwatch ships as a single static Linux binary with an embedded web UI and a RESTful API (OpenAPI-documented). No CLI is provided -- one can be generated from the OpenAPI spec if needed.

## Why Northwatch

Operating OVN at scale requires correlating information across two databases with 80+ tables, dozens of hypervisors, and thousands of logical flows. Existing tools (`ovn-nbctl`, `ovn-sbctl`, `ovn-trace`) are CLI-only, single-purpose, and require SSH access to the database nodes. Northwatch replaces this with a single, always-on service that provides:

- **Real-time streaming** of all NB/SB database changes via OVSDB monitor
- **Topology visualization** of logical and physical networks
- **Cross-database correlation** (e.g. NB Logical_Switch_Port -> SB Port_Binding -> Chassis)
- **Omnisearch** -- start with any IP, MAC, UUID, or name and find everything related across NB, SB and enrichment sources
- **Enrichment context** (e.g. port `abc123` belongs to instance `web-server-01` on project `production` via OpenStack, or pod `nginx-7b4f9` via Kubernetes)
- **Packet path tracing** equivalent to `ovn-trace` but via UI with visual pipeline rendering
- **Configuration validation** and drift detection
- **Historical snapshots** with diff and timeline navigation
- **Performance metrics** (config propagation delays, flow counts, DB sizes)
- **Multi-cluster support** for environments with multiple OVN deployments

## Architecture

```
                                    +------------------+
                                    |   Enrichment     |
                                    |   Providers      |
                                    | (OpenStack, K8s, |
                                    |  Static, ...)    |
                                    +--------+---------+
                                             |
                                             | Name Resolution
                                             |
+----------------+    TCP/OVSDB    +---------+---------+    HTTP/WS     +------------+
| OVN Northbound | <-------------> |                   | <-----------> |  Web UI     |
|   Database     |    monitor      |    Northwatch     |               | (Browser)   |
+----------------+                 |                   |               +------------+
                                   |  - In-Memory      |
+----------------+    TCP/OVSDB    |    Cache          |    HTTP/WS     +------------+
| OVN Southbound | <-------------> |  - Event Stream   | <-----------> | API Clients |
|   Database     |    monitor      |  - Analysis       |               |             |
+----------------+                 |    Engine         |               +------------+
                                   |  - Snapshot Store |
                                   |    (SQLite)       |
                                   +-------------------+
                                     Single Linux Binary
```

### Core Design Principles

1. **Real-time first** -- OVSDB `monitor` subscriptions keep a full in-memory cache of both NB and SB databases. All data is pushed, never polled. UI updates via WebSocket.
2. **Zero-copy correlation** -- The in-memory cache maintains pre-built indexes and cross-references between NB and SB entities (e.g. NB UUID -> SB Datapath_Binding, Logical_Switch_Port -> Port_Binding -> Chassis).
3. **Read-heavy, write-rare** -- Most operations are reads from the local cache. Writes (configuration changes) go through OVSDB transactions to the NB database and require explicit `write` capability.
4. **Single binary** -- Go binary with embedded static assets (UI) and embedded SQLite for snapshots. No external dependencies, no external database, no runtime.
5. **Enrichment-agnostic** -- Core functionality works without any enrichment provider. OpenStack, Kubernetes and other providers are pluggable and optional.
6. **Search-driven** -- Every entity is searchable. Omnisearch is the primary entry point for debugging workflows.
7. **HA-aware** -- Supports clustered OVSDB (Raft) with multiple endpoints, automatic failover, and cluster health monitoring.

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go (latest stable) |
| OVSDB Client | [libovsdb](https://github.com/ovn-kubernetes/libovsdb) |
| HTTP Router | stdlib `net/http` (Go 1.22+ routing) |
| WebSocket | `nhooyr.io/websocket` |
| Snapshot Store | Embedded SQLite (`modernc.org/sqlite` -- pure Go, no CGO) |
| API Spec | OpenAPI 3.1 |
| UI | Embedded SPA (Svelte + TypeScript) |
| OpenStack SDK | `github.com/gophercloud/gophercloud/v2` |
| Build | Single static binary via `go build` with embedded assets (`embed.FS`) |

## OVN Connectivity

Northwatch connects to both OVN databases using the OVSDB management protocol (RFC 7047) over TCP. It supports single-node and clustered (Raft) deployments.

| Connection | Default Port | Purpose |
|---|---|---|
| Northbound DB | `tcp:<host>:6641` | Logical network intent (switches, routers, ACLs, NAT, LB, ...) |
| Southbound DB | `tcp:<host>:6642` | Physical realization (chassis, port bindings, logical flows, ...) |

### Clustered OVSDB (Raft) Support

In production, OVN databases typically run as a 3- or 5-node Raft cluster. Northwatch handles this:

- **Multiple endpoints**: Configure a list of OVSDB endpoints per database; Northwatch connects to the current leader and fails over automatically
- **Cluster health monitoring**: Track leader/follower status, Raft term, log index, and connection state for each cluster member
- **Connection tracking**: Monitor which ovn-controllers are connected to the SB database, detect disconnected hypervisors

### Startup Sequence

On startup, Northwatch:

1. Connects to both NB and SB databases (with automatic leader discovery for Raft clusters)
2. Issues `monitor_all` for every table in both schemas
3. Populates the in-memory cache with the initial snapshot
4. Processes incremental updates as they arrive
5. Builds cross-reference indexes (NB <-> SB entity mapping)
6. Connects to configured enrichment providers (OpenStack, Kubernetes, etc.)
7. Takes an initial snapshot for the timeline

TLS is supported for all OVSDB connections.

## Capabilities

Northwatch uses a capability-based access model instead of exclusive operating modes. Capabilities can be combined freely and are selected at the UI/API level. There is no built-in user management or authentication -- access control should be handled at the network/reverse-proxy level.

| Capability | Description | Default |
|---|---|---|
| `read` | Browse NB/SB database contents, view topology, search, monitor real-time changes | Always on |
| `debug` | Packet tracing, flow diffs, config propagation analysis, port binding diagnostics, stale entry detection | On |
| `write` | Create, update, delete NB entities via OVSDB transactions. Requires explicit opt-in. | Off |

The default capability set is `read+debug`. The `write` capability must be explicitly enabled in the configuration and adds audit logging, confirmation flows, and snapshot-before-write safeguards.

### Read Capabilities

Available to all users:

- Browse NB and SB database contents
- View logical and physical topology
- Monitor real-time changes
- Search and filter across all entities (Omnisearch)
- Correlate OVN entities with enrichment provider names
- View telemetry and metrics
- Browse snapshots and timeline

### Debug Capabilities

Extended diagnostics:

- Logical flow pipeline visualization per datapath
- Packet path tracing (OVN-trace equivalent)
- Config propagation analysis (`nb_cfg` tracking per chassis)
- Port binding diagnostics (unbound ports, chassis mismatches)
- Stale entry detection (old MAC bindings, orphaned bindings)
- BFD session monitoring
- Controller event inspection
- Flow diff (real-time change tracking)
- Connectivity checker (path analysis between two logical ports)

### Write Capabilities

Configuration operations (requires explicit enablement):

- Create, update, delete logical switches, routers, ports
- Manage ACLs and security policies
- Configure NAT rules
- Manage load balancers
- Modify static routes and routing policies
- Set QoS and metering policies

Write safeguards:
- **Audit log**: Every mutation is logged with timestamp, previous state, and new state
- **Preview mode**: Show expected SB-side effects before applying (`terraform plan`-style)
- **Snapshot before write**: Automatic snapshot before every write operation for rollback
- **Explicit confirmation**: API requires a confirmation token; UI shows a diff preview with confirm/cancel

## Omnisearch

Omnisearch is the primary entry point for debugging workflows. A single search field queries across all data sources and returns grouped, navigable results.

### How It Works

- **Input recognition**: Automatically detects the search type -- IPv4/IPv6 address, MAC address, UUID (full or partial), or free-text name
- **Cross-source search**: Queries NB tables, SB tables, and enrichment provider caches in parallel
- **Grouped results**: Results are grouped by entity type (Logical Switch, Port, Router, Chassis, etc.) with enrichment context inline
- **Entity profile**: From any result, navigate to a full entity profile showing all relationships (NB entity -> SB counterpart -> Chassis -> Enrichment data)

### API

```
GET /api/v1/search?q=10.0.0.42        # Search by IP
GET /api/v1/search?q=fa:16:3e:aa:bb   # Search by MAC (partial)
GET /api/v1/search?q=web-server-01     # Search by name
GET /api/v1/search?q=a1b2c3d4          # Search by UUID fragment
```

## Core Data Views

These are the primary entities Northwatch exposes as first-class, browsable, searchable, real-time views. Each view combines data from NB, SB and enrichment providers into a single enriched representation.

### Chassis

Central view of the physical infrastructure.

- **List all chassis** with hostname, encapsulation type/IP, transport zones, registration status
- **Per-chassis detail**: hosted ports (Port_Bindings where `chassis` matches), local flow count, gateway role, HA group membership
- **Config realization status**: `nb_cfg` in Chassis_Private vs global `nb_cfg` -- shows how far behind each hypervisor is, with timestamp delta
- **BFD sessions**: All BFD sessions originating from or terminating at a chassis, with live status (up/down/init)
- **Encap health**: Geneve/VXLAN/STT tunnel endpoints, reachability
- **Enrichment**: Resolve chassis hostname to hypervisor details (e.g. Nova hypervisor: vCPU count, memory, running instances; or Kubernetes node: capacity, pods)

### Logical Flows

The heart of OVN's data plane -- compiled pipeline rules in the SB database.

- **Browse all Logical_Flows** with filtering by datapath, pipeline (ingress/egress), table_id, priority
- **Pipeline view**: For a selected datapath, render the full ingress and egress pipeline as an ordered table sequence (table 0 -> N), showing priority, match expression, actions, and flow description
- **Flow count per datapath**: Identify complex datapaths with excessive flow counts
- **Logical_DP_Group analysis**: Show which datapaths share flow groups, measure sharing efficiency
- **Diff view**: When a flow changes (via OVSDB monitor), show old vs new match/actions inline
- **Search**: Full-text search across match expressions and actions (e.g. find all flows matching a specific IP or MAC)

### Port Bindings

The mapping from logical ports to physical chassis.

- **List all Port_Bindings** with logical port name, type, datapath, chassis, tunnel key, MAC, status
- **Binding chain**: For any port, show the full NB -> SB chain: Logical_Switch_Port -> Port_Binding -> Chassis -> Encap
- **Unbound ports**: Filter for ports without a chassis (VIF not yet plugged or ovn-controller not claiming)
- **Chassis placement**: Group ports by chassis to see the workload distribution
- **Enrichment**: Resolve `iface-id` to port/instance/network name via the active enrichment provider

### Datapath Bindings

Logical datapaths and their tunnel key assignments.

- **List all Datapath_Bindings** with tunnel key, type, associated NB entity (via `nb_uuid` back-reference)
- **Per-datapath drill-down**: Linked Port_Bindings, Logical_Flows, Multicast_Groups
- **Enrichment**: Map datapath -> NB Logical_Switch/Router -> network/router name

### MAC Bindings & FDB

Dynamic and static address resolution state.

- **MAC_Bindings**: IP-to-MAC learned via ARP/ND, with timestamps, logical port, datapath
- **Static_MAC_Bindings**: Manually configured IP-to-MAC entries
- **FDB entries**: Port-to-MAC forwarding database with timestamps
- **Stale detection**: Highlight entries with old timestamps or referencing removed ports

### ACLs & Security

Security policy state across NB and SB.

- **ACL browser**: All ACLs with priority, direction, match, action, log settings, associated entity (switch or port group)
- **Port Group view**: Named groups of ports with their ACL sets -- maps directly to security groups in OpenStack or network policies in Kubernetes
- **Address Set view**: Named IP sets used in ACL match expressions -- maps to security group remote references
- **Security policy correlation**: Given a security group UUID (OpenStack) or network policy name (Kubernetes), show the corresponding Port_Group and all its ACL rules
- **Conflict detection**: Identify shadowed rules (lower-priority rule can never match because a higher-priority rule already catches all traffic)

### NAT & Floating IPs

NAT rules on logical routers.

- **List all NAT rules** with type (SNAT/DNAT/DNAT_AND_SNAT), external IP, logical IP, router, gateway port
- **Floating IP mapping**: For DNAT_AND_SNAT entries, show enrichment details (OpenStack floating IP, associated instance, network)
- **SNAT overview**: Per-router SNAT rules with external IP pools

### Load Balancers

VIP-to-backend mappings with health state.

- **List all Load_Balancers** with name, VIPs, protocol, associated switches/routers
- **Backend health**: Service_Monitor status per backend (from SB), health check configuration
- **Traffic distribution**: ECMP next-hop entries from SB

## Feature Areas

### 1. Analyzer

Deep inspection and correlation of OVN state.

- **Cross-DB correlation**: Map NB entities to their SB counterparts (Logical_Switch -> Datapath_Binding, Logical_Switch_Port -> Port_Binding)
- **Config propagation tracking**: Monitor `nb_cfg` / `sb_cfg` / `hv_cfg` across NB_Global, SB_Global, and every Chassis_Private to detect stuck or lagging hypervisors
- **Flow analysis**: Count Logical_Flows per datapath, identify complex datapaths, analyze Logical_DP_Group sharing efficiency
- **Security audit**: Enumerate all ACLs with priorities, detect shadowed/conflicting rules, map security policies to port groups
- **Stale entry detection**: Identify MAC_Bindings with old timestamps, FDB entries for removed ports, orphaned Port_Bindings
- **Capacity metrics**: Count logical switches, ports, routers, ACLs, NAT rules, load balancers -- track growth over time via snapshots

### 2. Visualizer

Graphical representation of the OVN topology and state.

- **Logical topology graph**: Logical switches and routers as nodes, ports as edges, with live status coloring
- **Physical topology**: Chassis nodes with their hosted ports, tunnel mesh overlay
- **Combined view**: Logical topology overlaid on physical placement (which ports on which chassis)
- **Gateway topology**: HA chassis groups, active/standby state, BFD session health
- **NAT/Floating IP map**: External-to-internal IP mappings with router association
- **Load balancer view**: VIP -> backend pool with health check status
- **Flow pipeline**: Tabular/visual representation of the logical ingress/egress pipeline stages for a selected datapath
- **Export**: All visualizations exportable as SVG/PNG for documentation and incident reports

### 3. Debugger

Troubleshooting tools for OVN network issues.

- **Packet path tracer**: Equivalent to `ovn-trace` -- simulate a packet through the logical flow pipeline, showing each table match/action. Primary implementation evaluates SB Logical_Flows against the cached state. Fallback: delegate to `ovn-trace` on the DB node via optional SSH/agent integration (see Technical Risks).
- **Port binding inspector**: For a given port, show the full chain: NB LSP -> SB Port_Binding -> Chassis -> Encap, highlighting mismatches or missing bindings
- **Config lag detector**: Per-chassis breakdown of config realization delay using `nb_cfg_timestamp` in Chassis_Private
- **Controller events**: Stream and filter Controller_Event entries from the SB database
- **Connectivity checker**: Given two logical ports, determine the expected path and identify potential blocking ACLs or missing routes
- **Flow diff**: Real-time diff when Logical_Flows change -- highlight what ovn-northd recompiled and why
- **Trace export**: Packet traces exportable as shareable links or JSON/text for inclusion in incident reports

### 4. Configurator (Write Capability)

Safe configuration interface for OVN NB database. Requires the `write` capability to be explicitly enabled.

- **CRUD operations** on all NB entities via OVSDB transactions
- **Validation** before applying changes (e.g. verify referenced entities exist, check for IP conflicts)
- **Preview mode**: Show what SB changes a NB modification would produce (by analyzing ovn-northd compilation patterns) -- similar to `terraform plan`
- **Snapshot before write**: Automatic snapshot before every mutation for point-in-time rollback
- **Batch operations**: Apply multiple related changes as a single OVSDB transaction
- **Audit log**: Every write operation logged with timestamp, user context (from reverse proxy headers), previous state, and new state

### 5. Snapshots & Timeline

Historical state tracking using embedded SQLite.

- **Automatic snapshots**: Periodic snapshots of full NB+SB state (configurable interval, default: 1 hour)
- **Event-triggered snapshots**: Snapshot before write operations, on significant topology changes, or on-demand via API/UI
- **Snapshot diff**: Compare any two snapshots side-by-side -- see what changed between two points in time
- **Timeline UI**: Visual timeline of snapshots and events with navigation slider; click any point to view historical state
- **Event persistence**: All OVSDB change events stored in SQLite with full before/after state (ring-buffer with configurable retention, default: 7 days)
- **Rollback**: From any snapshot, generate the OVSDB transactions needed to restore that state (requires `write` capability to execute)
- **Export**: Snapshots exportable as JSON dumps for offline analysis or sharing

### 6. Telemetry

Metrics collection from the OVSDB event stream.

- **Database metrics**: Table row counts, transaction rates, monitor update rates for both NB and SB
- **Entity metrics**: Logical flows per datapath, ports per switch, ACLs per port group
- **Timing metrics**: Config propagation latency (NB change to full hypervisor realization)
- **Chassis metrics**: Per-chassis port count, flow count, BFD status summary, config realization lag
- **Health check metrics**: Load balancer backend health, service monitor results
- **Cluster metrics**: OVSDB Raft cluster health -- leader status, follower lag, connection count
- **Trend data**: Key metrics tracked over time via snapshots for capacity planning
- **Prometheus endpoint**: `/metrics` endpoint for scraping by external monitoring systems

### 7. Monitoring

Continuous health assessment and alerting.

- **Database connectivity**: NB and SB connection status with automatic reconnection and Raft failover
- **Cluster health**: OVSDB Raft leader/follower status, log replication lag, cluster membership changes
- **Chassis health**: Detect chassis that stop updating `nb_cfg` (potential ovn-controller failure), chassis appearing/disappearing
- **Port status**: Alert on ports stuck in `up=false` or missing chassis binding
- **BFD failures**: Alert on BFD session state transitions (up -> down)
- **HA failover events**: Detect and log gateway chassis failovers
- **Flow count anomalies**: Alert on sudden spikes in Logical_Flow count (could indicate ovn-northd loop or misconfiguration)
- **Threshold alerts**: Configurable thresholds for flow counts, config lag, DB sizes
- **Event log**: Persistent event log in SQLite (replaces volatile in-memory ring buffer) with configurable retention

## Enrichment Providers

Northwatch resolves OVN UUIDs to human-readable names via pluggable enrichment providers. The core functionality works without any provider configured -- enrichment is always additive.

### Provider Interface

Each provider implements a common interface:

```go
type EnrichmentProvider interface {
    Name() string
    Resolve(ctx context.Context, entityType string, id string) (*Enrichment, error)
    BulkResolve(ctx context.Context, entityType string, ids []string) (map[string]*Enrichment, error)
    Watch(ctx context.Context, updates chan<- EnrichmentUpdate) error
}
```

### OpenStack Provider

The primary enrichment provider, connecting via Gophercloud.

#### Name Resolution Mapping

| OVN Entity | external_ids Key | OpenStack API | Resolved Name |
|---|---|---|---|
| Logical_Switch | `neutron:network_name` | Neutron `GET /networks` | Network name |
| Logical_Switch_Port | `neutron:port_name` | Neutron `GET /ports` | Port name / device info |
| Logical_Switch_Port | `neutron:device_owner` | -- | Device type (compute, dhcp, router) |
| Logical_Switch_Port | `neutron:device_id` | Nova `GET /servers` | Instance name |
| Logical_Router | `neutron:router_name` | Neutron `GET /routers` | Router name |
| NAT (FIP) | `neutron:fip_id` | Neutron `GET /floatingips` | Floating IP details |
| Port_Binding (SB) | `iface-id` | Neutron `GET /ports` | Port/Instance info |
| Chassis | hostname | Nova `GET /os-hypervisors` | Hypervisor details |

#### Configuration

```yaml
enrichment:
  providers:
    - type: openstack
      enabled: true
      auth_url: "https://keystone.example.com:5000/v3"
      project_name: "admin"
      user_domain_name: "Default"
      project_domain_name: "Default"
      username: "northwatch"
      password: "${OS_PASSWORD}"  # Env var substitution
      # Or use application credentials:
      # application_credential_id: "..."
      # application_credential_secret: "..."
      cache_ttl: 300s
      regions: ["RegionOne"]
```

### Kubernetes Provider (Planned)

For ovn-kubernetes deployments, resolving OVN entities to Kubernetes resources:

| OVN Entity | Kubernetes Resource | Resolved Name |
|---|---|---|
| Logical_Switch | Namespace | Namespace name |
| Logical_Switch_Port | Pod | Pod name, namespace, node |
| Logical_Router | Node / Service | Node name, service details |
| Chassis | Node | Node name, capacity, conditions |

### Caching Strategy

All enrichment providers use a common caching layer:

- Initial bulk load on startup
- TTL-based refresh (configurable per provider, default: 5 minutes)
- Event-driven invalidation when OVN entities change (new port -> fetch enrichment details)
- Manual refresh via API/UI
- Async resolution: cache miss returns raw UUID immediately, enriched data arrives via WebSocket update

## API Design

RESTful JSON API with OpenAPI 3.1 specification. All endpoints are prefixed with `/api/v1`.

### Key Endpoint Groups

```
GET  /api/v1/capabilities                  # Active capabilities
PUT  /api/v1/capabilities                  # Update capabilities (if allowed)

# Omnisearch
GET  /api/v1/search?q=<query>             # Global search across all data sources

# NB Database
GET  /api/v1/nb/logical-switches          # List logical switches
GET  /api/v1/nb/logical-switches/:uuid    # Get logical switch details
GET  /api/v1/nb/logical-routers           # List logical routers
GET  /api/v1/nb/logical-router-ports      # List logical router ports
GET  /api/v1/nb/acls                      # List ACLs
GET  /api/v1/nb/nat                       # List NAT rules
GET  /api/v1/nb/load-balancers            # List load balancers
GET  /api/v1/nb/dhcp-options              # List DHCP options
GET  /api/v1/nb/address-sets              # List address sets
GET  /api/v1/nb/port-groups               # List port groups
# ... CRUD for all NB tables (POST/PUT/DELETE require write capability)

# SB Database -- Chassis
GET  /api/v1/sb/chassis                   # List all chassis (enriched)
GET  /api/v1/sb/chassis/:name             # Chassis detail
GET  /api/v1/sb/chassis/:name/ports       # Ports bound to this chassis
GET  /api/v1/sb/chassis/:name/flows       # Logical flows relevant to this chassis
GET  /api/v1/sb/chassis/:name/config      # Config realization status (nb_cfg tracking)

# SB Database -- Logical Flows
GET  /api/v1/sb/logical-flows             # List logical flows (filterable)
GET  /api/v1/sb/logical-flows?datapath=:uuid          # Filter by datapath
GET  /api/v1/sb/logical-flows?pipeline=ingress        # Filter by pipeline
GET  /api/v1/sb/logical-flows?table_id=0              # Filter by table
GET  /api/v1/sb/logical-flows?match=10.0.0.1          # Search in match expressions
GET  /api/v1/sb/logical-flows/pipeline/:datapath_uuid # Full pipeline view for a datapath
GET  /api/v1/sb/logical-flows/stats                   # Flow counts per datapath
GET  /api/v1/sb/logical-dp-groups                     # DP group sharing analysis

# SB Database -- Port Bindings
GET  /api/v1/sb/port-bindings             # List all port bindings (enriched)
GET  /api/v1/sb/port-bindings/:uuid       # Port binding detail with full NB->SB chain
GET  /api/v1/sb/port-bindings/unbound     # Ports without chassis binding
GET  /api/v1/sb/port-bindings/by-chassis/:name  # Ports on a specific chassis

# SB Database -- Datapath Bindings
GET  /api/v1/sb/datapath-bindings         # List datapath bindings
GET  /api/v1/sb/datapath-bindings/:uuid   # Datapath detail (linked flows, ports, multicast groups)

# SB Database -- Other
GET  /api/v1/sb/mac-bindings              # MAC bindings (dynamic ARP/ND)
GET  /api/v1/sb/mac-bindings/stale        # Stale MAC bindings
GET  /api/v1/sb/fdb                       # Forwarding database
GET  /api/v1/sb/controller-events         # Controller events
GET  /api/v1/sb/service-monitors          # Load balancer health checks
GET  /api/v1/sb/bfd-sessions              # BFD session states
GET  /api/v1/sb/multicast-groups          # Multicast group memberships

# Cross-database correlation
GET  /api/v1/topology/logical             # Logical topology graph
GET  /api/v1/topology/physical            # Physical topology (chassis + bindings)
GET  /api/v1/topology/combined            # Combined logical-physical view

# Debugging (requires debug capability)
POST /api/v1/debug/trace                  # Packet path trace
GET  /api/v1/debug/config-propagation     # nb_cfg tracking per chassis
GET  /api/v1/debug/config-propagation/:chassis  # Single chassis propagation detail
GET  /api/v1/debug/unbound-ports          # Ports without chassis binding
GET  /api/v1/debug/stale-entries          # Stale MAC/FDB entries
GET  /api/v1/debug/flow-diff              # Recent logical flow changes (old vs new)
GET  /api/v1/debug/connectivity?from=:uuid&to=:uuid  # Path analysis between two ports

# Snapshots & Timeline
GET  /api/v1/snapshots                    # List snapshots
POST /api/v1/snapshots                    # Create on-demand snapshot
GET  /api/v1/snapshots/:id               # Get snapshot details
GET  /api/v1/snapshots/:id/state          # Full state at snapshot time
GET  /api/v1/snapshots/diff?from=:id&to=:id  # Diff between two snapshots
GET  /api/v1/events?since=<timestamp>&until=<timestamp>  # Historical events
GET  /api/v1/events/stream                # SSE stream for live events

# Telemetry
GET  /api/v1/telemetry/summary            # Overview metrics
GET  /api/v1/telemetry/flows              # Flow count metrics
GET  /api/v1/telemetry/propagation        # Config propagation timing
GET  /api/v1/telemetry/cluster            # OVSDB cluster health
GET  /metrics                             # Prometheus metrics

# Enrichment
GET  /api/v1/enrichment/resolve/:uuid     # Resolve UUID via active providers
GET  /api/v1/enrichment/cache/status      # Cache status per provider
POST /api/v1/enrichment/cache/refresh     # Force cache refresh

# Export
GET  /api/v1/export/topology?format=svg   # Export topology as SVG/PNG
GET  /api/v1/export/trace/:id?format=json # Export packet trace
GET  /api/v1/export/snapshot/:id          # Export snapshot as JSON dump

# WebSocket
WS   /api/v1/ws/events                    # Real-time event stream (all DB changes)
WS   /api/v1/ws/events?table=Port_Binding # Filtered event stream
```

### WebSocket Event Stream

The WebSocket endpoint streams real-time OVSDB changes to connected clients:

```json
{
  "database": "OVN_Northbound",
  "table": "Logical_Switch_Port",
  "event": "update",
  "timestamp": "2026-03-19T10:30:00.123Z",
  "uuid": "a1b2c3d4-...",
  "old": { "up": false },
  "new": { "up": true },
  "enrichment": {
    "provider": "openstack",
    "port_name": "web-server-01-eth0",
    "device_name": "web-server-01",
    "network_name": "internal-net",
    "project_name": "production"
  }
}
```

Clients can subscribe to specific tables, databases, or event types via query parameters or subscription messages.

## Configuration

```yaml
# northwatch.yaml

server:
  listen: ":8080"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

# Clusters -- Northwatch can monitor multiple OVN deployments
clusters:
  - name: "production"
    ovn:
      northbound:
        # Single endpoint or list for Raft clusters
        addresses:
          - "tcp:ovn-nb-1.example.com:6641"
          - "tcp:ovn-nb-2.example.com:6641"
          - "tcp:ovn-nb-3.example.com:6641"
        tls:
          enabled: false
          cert_file: ""
          key_file: ""
          ca_file: ""
      southbound:
        addresses:
          - "tcp:ovn-sb-1.example.com:6642"
          - "tcp:ovn-sb-2.example.com:6642"
          - "tcp:ovn-sb-3.example.com:6642"
        tls:
          enabled: false
          cert_file: ""
          key_file: ""
          ca_file: ""
    enrichment:
      providers:
        - type: openstack
          enabled: true
          auth_url: "https://keystone.example.com:5000/v3"
          project_name: "admin"
          user_domain_name: "Default"
          project_domain_name: "Default"
          username: "northwatch"
          password: "${OS_PASSWORD}"
          cache_ttl: 300s
          regions: ["RegionOne"]

  - name: "staging"
    ovn:
      northbound:
        addresses: ["tcp:ovn-nb.staging.example.com:6641"]
      southbound:
        addresses: ["tcp:ovn-sb.staging.example.com:6642"]

capabilities:
  debug: true
  write: false  # Explicitly opt-in

snapshots:
  enabled: true
  interval: 1h
  retention: 168h  # 7 days
  db_path: "/var/lib/northwatch/snapshots.db"

events:
  retention: 168h  # 7 days
```

Also configurable via environment variables: `NORTHWATCH_CLUSTERS_0_OVN_NB_ADDRESSES`, etc.

## Project Structure

```
northwatch/
  cmd/
    northwatch/
      main.go                 # Entry point
  internal/
    config/                   # Configuration loading (YAML + env vars)
    ovsdb/
      client.go               # libovsdb client wrapper, connection management, Raft failover
      nb/                     # Generated NB schema models (via libovsdb modelgen)
      sb/                     # Generated SB schema models (via libovsdb modelgen)
      cache.go                # Cross-DB in-memory cache with indexes
      monitor.go              # OVSDB monitor subscriptions and event routing
      cluster.go              # Raft cluster health tracking
    enrichment/
      provider.go             # EnrichmentProvider interface
      cache.go                # Common caching layer for all providers
      openstack/
        provider.go           # OpenStack enrichment (Gophercloud)
      kubernetes/
        provider.go           # Kubernetes enrichment (client-go) [planned]
      static/
        provider.go           # Static mapping file provider
    search/
      omnisearch.go           # Global search engine across all data sources
      index.go                # Search indexes (trigram, prefix, etc.)
    api/
      server.go               # HTTP server setup, middleware, routing
      handler/
        nb.go                 # NB database endpoints
        sb.go                 # SB database endpoints
        topology.go           # Topology endpoints
        debug.go              # Debug/trace endpoints
        telemetry.go          # Telemetry endpoints
        enrichment.go         # Enrichment resolution endpoints
        capabilities.go       # Capability management
        search.go             # Omnisearch endpoint
        snapshots.go          # Snapshot and timeline endpoints
        export.go             # Export endpoints (SVG, PNG, JSON)
        ws.go                 # WebSocket event stream
      middleware/
        capabilities.go       # Capability-based access control
        audit.go              # Audit logging for write operations
    engine/
      analyzer.go             # Cross-DB analysis, stale detection, security audit
      tracer.go               # Packet path tracing (ovn-trace equivalent)
      correlator.go           # NB <-> SB entity correlation
    snapshots/
      store.go                # SQLite snapshot storage
      diff.go                 # Snapshot comparison
      timeline.go             # Timeline navigation
    telemetry/
      collector.go            # Metrics collection from event stream
      prometheus.go           # Prometheus exporter
    monitor/
      health.go               # Health checks, alerting
      events.go               # Persistent event log (SQLite-backed)
    cluster/
      manager.go              # Multi-cluster management
  ui/
    embed.go                  # embed.FS for static assets
    frontend/                 # Svelte + TypeScript SPA
  api/
    openapi.yaml              # OpenAPI 3.1 specification
  configs/
    northwatch.example.yaml   # Example configuration
  Makefile
  go.mod
  go.sum
```

## Performance Considerations

### In-Memory Cache

The entire NB and SB database state lives in memory. For a large deployment:

| Metric | Typical Scale | Memory Estimate |
|---|---|---|
| Logical Switches | 1,000 | ~1 MB |
| Logical Switch Ports | 50,000 | ~50 MB |
| Logical Routers | 500 | ~1 MB |
| ACLs | 100,000 | ~100 MB |
| Logical Flows (SB) | 500,000 | ~500 MB |
| Port Bindings (SB) | 50,000 | ~50 MB |
| Chassis | 500 | ~1 MB |
| **Subtotal (cache)** | | **~700 MB** |
| Snapshot DB (7 days) | | ~2-5 GB |
| **Total** | | **~3-6 GB** |

This is acceptable for a dedicated monitoring service. The snapshot database size depends on change frequency and can be controlled via retention settings.

### Event Processing

- OVSDB `monitor` delivers changes as JSON diffs -- only modified columns are sent
- Cache updates are applied lock-free using copy-on-write where possible
- WebSocket fan-out uses per-client goroutines with backpressure
- Enrichment resolution is async and non-blocking (cache miss returns UUID, enriched later)
- Snapshot writes are batched and run in a background goroutine to avoid blocking the event pipeline

### Startup

- Initial `monitor_all` response can be large (especially Logical_Flows)
- Startup is I/O-bound, not CPU-bound -- parallelized across NB and SB connections
- Enrichment provider bulk load runs concurrently with OVSDB sync
- Health endpoint reports `ready` only after initial sync completes
- For multi-cluster setups, each cluster syncs independently in parallel

## Technical Risks

### Packet Tracer Complexity

Implementing an `ovn-trace` equivalent purely from cached state is the highest technical risk in this project. It requires:

- A full parser for OVN match expressions (OpenFlow-like syntax with OVN extensions)
- An evaluator that can walk the logical flow pipeline, applying match/action semantics correctly
- Handling of all action types (next, output, drop, ct_next, ct_commit, etc.)
- Correct modeling of conntrack state, NAT, and load balancer DNAT

**Mitigation strategy**:
1. Start with a simplified tracer that handles the most common flow patterns (basic L2/L3 forwarding, simple ACLs)
2. Incrementally add support for complex actions (conntrack, NAT, LB)
3. Provide a fallback mode that delegates to `ovn-trace` on the DB node via optional SSH integration or a lightweight agent
4. Clearly indicate in the UI which flow actions are fully evaluated vs approximated

### OVSDB Scale

For very large deployments (100k+ ports), the initial `monitor_all` response for the Southbound database can be hundreds of megabytes. Mitigations:

- Stream-parse the initial response instead of buffering entirely in memory
- Consider `monitor_cond` (conditional monitoring) for tables where full monitoring is not needed
- Implement table-level monitoring toggles in configuration

## Building

```bash
# Generate OVSDB models from schema files
make generate

# Build the binary
make build
# -> bin/northwatch (static Linux binary)

# Build with embedded UI
make build-ui  # builds frontend assets
make build     # embeds them via go:embed
```

## Running

```bash
# Minimal (single cluster, no enrichment)
./northwatch \
  --ovn-nb-address tcp:10.0.0.1:6641 \
  --ovn-sb-address tcp:10.0.0.1:6642

# With Raft cluster endpoints
./northwatch \
  --ovn-nb-address tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641 \
  --ovn-sb-address tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642

# With OpenStack enrichment
./northwatch \
  --ovn-nb-address tcp:10.0.0.1:6641 \
  --ovn-sb-address tcp:10.0.0.1:6642 \
  --openstack-auth-url https://keystone.example.com:5000/v3 \
  --openstack-username northwatch \
  --openstack-password secret \
  --openstack-project-name admin

# With config file (recommended for multi-cluster)
./northwatch --config /etc/northwatch/northwatch.yaml

# Enable write capability
./northwatch --config northwatch.yaml --enable-write
```

Access the UI at `http://localhost:8080` and the API at `http://localhost:8080/api/v1`.

## Roadmap

### Phase 1 -- Foundation: Useful Database Browser

Goal: Replace `ovn-nbctl` / `ovn-sbctl` for daily inspection tasks.

- [ ] Project scaffolding, Go module, libovsdb model generation
- [ ] OVSDB client: connect to NB + SB (single endpoint), monitor all tables, populate cache
- [ ] Basic REST API: list/get for all NB and SB tables with filtering
- [ ] Capability system (`read` + `debug`)
- [ ] Minimal embedded UI: table browser with search/filter
- [ ] Omnisearch (cross-table, cross-database)

### Phase 2 -- Correlation & Enrichment

Goal: Make OVN data understandable by humans.

- [ ] Cross-DB correlation engine (NB <-> SB mapping)
- [ ] Enrichment provider interface + OpenStack provider (Gophercloud, name resolution, caching)
- [ ] Binding chain view (NB -> SB -> Chassis, enriched)
- [ ] Entity profile pages in UI

### Phase 3 -- Real-time & Visualization

Goal: Live operational dashboard.

- [ ] WebSocket event stream
- [ ] Real-time event feed in UI
- [ ] Topology API (logical, physical, combined)
- [ ] Interactive topology visualization (Svelte)
- [ ] Config propagation tracking

### Phase 4 -- Debugging

Goal: Replace SSH + CLI debugging workflow.

- [ ] Packet path tracer (simplified subset first, see Technical Risks)
- [ ] Flow pipeline visualization
- [ ] Flow diff (real-time change tracking)
- [ ] Connectivity checker
- [ ] Port binding diagnostics

### Phase 5 -- Telemetry & Monitoring

Goal: Always-on health monitoring.

- [ ] Prometheus metrics endpoint
- [ ] Alerting (chassis health, port status, BFD, flow count anomalies)
- [ ] OVSDB Raft cluster health monitoring
- [ ] Multiple OVSDB endpoints with failover

### Phase 6 -- History & Snapshots

Goal: Answer "what changed and when?"

- [ ] SQLite snapshot store
- [ ] Automatic and on-demand snapshots
- [ ] Snapshot diff
- [ ] Timeline UI
- [ ] Persistent event log with retention

### Phase 7 -- Configurator & Advanced

Goal: Safe write access and multi-environment support.

- [ ] Write capability with audit logging and safeguards
- [ ] Preview mode (terraform plan-style)
- [ ] Snapshot-before-write and rollback
- [ ] Multi-cluster support
- [ ] Export (SVG/PNG/JSON)
- [ ] Kubernetes enrichment provider
- [ ] OpenAPI spec generation and documentation

### Phase 8 -- Production Hardening

Goal: Ready for large-scale production deployments.

- [ ] Performance optimization for large deployments (100k+ ports)
- [ ] `monitor_cond` for selective table monitoring
- [ ] Comprehensive test suite (unit + integration with libovsdb test server)
- [ ] Security hardening (TLS everywhere, input validation)
- [ ] Packaging (systemd unit, container image)

## License

Apache License 2.0
