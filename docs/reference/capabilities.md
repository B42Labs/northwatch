# Capabilities

Northwatch advertises a set of **capabilities** at `/api/v1/capabilities`. The UI
reads this list to decide which views and actions to show. Capabilities are
additive — there are no exclusive operating modes — and there is no built-in
authentication; access control belongs at the network / reverse-proxy layer (see
[The capability model](/explanation/capability-model)).

## The endpoint

```bash
curl -s http://localhost:8080/api/v1/capabilities
```

```json
{
  "capabilities": ["read", "debug", "correlate", "realtime", "topology",
                   "flows", "telemetry", "alerts", "history", "openapi"],
  "mode": "live"
}
```

## Always-on capabilities

These are present on every live server:

| Capability | Covers |
|---|---|
| `read` | Browsing the NB/SB tables. |
| `debug` | Trace, connectivity, port diagnostics, ACL audit, stale-entry detection, flow diff. |
| `correlate` | The `correlated/*` views joining NB↔SB. |
| `realtime` | The WebSocket event stream. |
| `topology` | Logical, physical, gateway, NAT and load-balancer topology. |
| `flows` | Logical-flow views. |
| `telemetry` | Telemetry endpoints. |
| `alerts` | Alert rules and silences. |
| `history` | Events, snapshots and timeline. |
| `openapi` | The self-served OpenAPI spec and Swagger UI. |

## Conditional capabilities

These appear only when the matching feature is configured:

| Capability | Appears when |
|---|---|
| `enrich` | An enrichment provider is configured (OpenStack or Kubernetes). |
| `write` | `--write-enabled` is set. |
| `multi-cluster` | More than one cluster is configured. |
| `ovs` | Per-chassis Open_vSwitch visibility is configured (`--ovs-mgmt-addr-file`). |
| `snapshot` | The server is serving an offline snapshot (`--snapshot`). |

## Mode

`mode` is `live` for a normal server and `snapshot` when serving an offline
snapshot file. In `snapshot` mode the live-tracking capabilities still appear in
the list but the underlying subsystems (alerts, telemetry, flow diff, websocket)
are not active for the snapshot cluster — a snapshot is a frozen point in time.
When the server is offline, the response also includes a `snapshot` object with
the capture's `createdAt` and source addresses.

## Related

- [The capability model](/explanation/capability-model) — the reasoning.
- [HTTP API](/reference/api) — which routes each capability maps to.
