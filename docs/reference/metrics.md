# Prometheus metrics

Northwatch exposes Prometheus metrics at `/metrics` (always on). Alongside the
standard Go runtime and process collectors, it exports OVN-specific gauges and
its own HTTP server metrics. Metrics cover the **default** cluster only.

See [Scrape Prometheus metrics](/how-to/scrape-prometheus-metrics) for scrape
configuration.

## OVN & connection metrics

| Metric | Type | Meaning |
|---|---|---|
| `northwatch_ovsdb_connected` | gauge | Whether an OVSDB endpoint is connected. |
| `northwatch_ovsdb_connections` | gauge | OVSDB `Connection` table entries, labeled by `database` and connected state. |
| `northwatch_ovsdb_table_rows` | gauge | Row count per monitored table. |
| `northwatch_logical_flows_total` | gauge | Total Southbound logical flows. |
| `northwatch_port_bindings_total` | gauge | Total port bindings. |
| `northwatch_bfd_sessions` | gauge | BFD session counts by state. |

## Config-propagation metrics

These track how far each hypervisor's realized config lags the desired config
(`nb_cfg` / `sb_cfg` / `hv_cfg`):

| Metric | Type | Meaning |
|---|---|---|
| `northwatch_nb_cfg` | gauge | Global Northbound `nb_cfg` sequence. |
| `northwatch_sb_cfg` | gauge | Configuration generation applied by northd (`NB_Global.sb_cfg`). |
| `northwatch_sb_nb_cfg` | gauge | Southbound view of `nb_cfg`. |
| `northwatch_hv_cfg` | gauge | Hypervisor `hv_cfg` sequence. |
| `northwatch_chassis_nb_cfg` | gauge | Per-chassis realized `nb_cfg`. |
| `northwatch_chassis_nb_cfg_lag` | gauge | Per-chassis config-realization lag. |
| `northwatch_chassis_port_count` | gauge | Ports bound per chassis. |

## HTTP server metrics

| Metric | Type | Meaning |
|---|---|---|
| `northwatch_http_requests_total` | counter | HTTP requests served. Labeled by `method`, `status` and `path` — where `path` is the **matched route pattern** (e.g. `/api/v1/nb/logical-switches/{uuid}`), not the raw URL. Requests that match no route collapse onto a single `unmatched` label, so the label set stays bounded. |
| `northwatch_http_request_duration_seconds` | histogram | Request latency. |

## Standard collectors

The registry also includes Go's `GoCollector` (`go_*`) and `ProcessCollector`
(`process_*`) for runtime and process metrics.

::: info
Metric label sets and exact help text are best read from a live scrape:
`curl -s http://localhost:8080/metrics | grep northwatch_`.
:::
