# Configure alerts

Northwatch continuously evaluates a set of built-in alert rules against the live
OVSDB event stream and can push firing alerts to webhooks. Alerts are part of the
default capability set on a live server (they are not available in offline
snapshot mode).

## Built-in rules

The alert engine ships with these rules enabled:

| Rule name | Fires when |
|---|---|
| `stale_chassis_config` | A chassis stops updating its config (`nb_cfg`) for too long |
| `port_down` | A port is stuck `up=false` |
| `unbound_port` | A logical port has no chassis binding |
| `bfd_down` | A BFD session transitions to down |
| `flow_count_anomaly` | `Logical_Flow` count spikes beyond a threshold |
| `ha_failover` | A gateway chassis failover is detected |

The rule name is what you reference when silencing or toggling a rule.

## Inspect alerts over the API

```bash
curl -s http://localhost:8080/api/v1/alerts          # currently firing alerts
curl -s http://localhost:8080/api/v1/alerts/rules    # configured rules
```

## Disable or re-enable a rule

Rules can be toggled at runtime without a restart. Like every mutating request,
this needs a bearer token (see
[Enable write operations](/how-to/enable-write-operations#authenticate-the-calls),
the same `--api-tokens` mechanism, with no `--write-enabled` required):

```bash
AUTH="Authorization: Bearer $NW_TOKEN"
curl -s -X PUT http://localhost:8080/api/v1/alerts/rules/flow_count_anomaly \
  -H "$AUTH" --data '{"enabled": false}'
```

An unknown rule name returns `404`.

## Silence an alert

A silence suppresses matching alerts for a duration without disabling the rule.
Creating and deleting silences are mutating requests, so they also need the
bearer token; listing does not:

```bash
curl -s -X POST http://localhost:8080/api/v1/alerts/silences -H "$AUTH" \
  --data '{"rule": "port_down", "duration": "2h", "comment": "maintenance on chassis-1"}'
curl -s http://localhost:8080/api/v1/alerts/silences
curl -s -X DELETE http://localhost:8080/api/v1/alerts/silences/<id> -H "$AUTH"
```

The body takes `rule` (a rule name), `matchers` (a map of alert labels to
match), `duration` (Go duration, default `1h`) and `comment`; at least one of
`rule` or `matchers` is required. Creation returns `201` with the stored
silence, including its `id` and expiry.

## Send alerts to webhooks

Point Northwatch at one or more webhook URLs (comma-separated) to receive
notifications when alerts fire:

```bash
./bin/northwatch \
  --ovn-nb-addr tcp:10.0.0.1:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642 \
  --alert-webhook-urls https://hooks.example.com/ovn,https://hooks.example.com/oncall
```

On startup the server logs `alert webhook notifications enabled` per cluster,
with the endpoint count. The flag can also be set via
`NORTHWATCH_ALERT_WEBHOOK_URLS`.

Each notification is a `POST` with `Content-Type: application/json` and
`User-Agent: northwatch-alertmanager` (10-second timeout per request). Firing
and resolved alerts arrive as separate payloads:

```json
{
  "status": "firing",
  "alerts": [
    {
      "rule": "port_down",
      "severity": "warning",
      "state": "firing",
      "message": "…",
      "labels": { "…": "…" },
      "fired_at": "2026-07-14T09:00:00Z"
    }
  ]
}
```

A non-2xx response is logged and the alert is not retried.

## See it in the lab

The lab's `make lab-sim` triggers real alerts: unbinding a VIF fires "unbound
port", and the gateway-failover simulation exercises the HA-failover path. See
[Run the local lab](/how-to/run-the-local-lab).
