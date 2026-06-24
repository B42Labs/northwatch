# Scrape Prometheus metrics

Northwatch exposes Prometheus metrics at `/metrics`, covering OVSDB connection
health, table row counts, config-propagation state, and its own HTTP server. The
endpoint is always on.

## Scrape it

```bash
curl -s http://localhost:8080/metrics | grep northwatch_
```

A minimal Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: northwatch
    static_configs:
      - targets: ['northwatch.example.com:8080']
```

## Liveness and readiness

For orchestrators, Northwatch also serves plain health probes:

```bash
curl -s http://localhost:8080/healthz   # liveness
curl -s http://localhost:8080/readyz    # readiness (ready after initial sync)
```

## What is exported

The custom collector exports OVN-specific gauges alongside the standard Go and
process collectors. Highlights:

| Metric | Meaning |
|---|---|
| `northwatch_ovsdb_connected` | Whether each OVSDB endpoint is connected |
| `northwatch_ovsdb_table_rows` | Row count per monitored table |
| `northwatch_logical_flows_total` | Total Southbound logical flows |
| `northwatch_port_bindings_total` | Total port bindings |
| `northwatch_chassis_nb_cfg_lag` | Per-chassis config-realization lag |
| `northwatch_bfd_sessions` | BFD session counts by state |
| `northwatch_http_requests_total` | Northwatch HTTP request counter |

For the complete list, see [Prometheus metrics](/reference/metrics).

::: info Single-cluster scope
Metrics are exported for the **default** cluster only. Per-cluster routes do not
register their own Prometheus collectors.
:::
