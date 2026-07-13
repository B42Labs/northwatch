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

::: warning /metrics is unauthenticated
`/metrics` is served on the main mux and needs **no** API token — the bearer
tokens gate mutation, not reading. With the default loopback bind only a local
scraper reaches it; when you expose Northwatch, restrict `/metrics` at the same
reverse proxy that protects the rest of the read surface. See [Deploy to
production](/how-to/deploy-production).
:::

::: info The `path` label is the route pattern
`northwatch_http_requests_total` and `northwatch_http_request_duration_seconds`
label requests with the route pattern the server matched (e.g.
`/api/v1/nb/logical-switches/{uuid}`), not the raw URL, and collapse unmatched
requests onto a single `unmatched` label. This keeps the number of time series
bounded by the number of routes.
:::

::: info Single-cluster scope
Metrics are exported for the **default** cluster only. Per-cluster routes do not
register their own Prometheus collectors.
:::
