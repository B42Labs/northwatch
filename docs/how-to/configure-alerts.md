# Configure alerts

Northwatch continuously evaluates a set of built-in alert rules against the live
OVSDB event stream and can push firing alerts to webhooks. Alerts are part of the
default capability set on a live server (they are not available in offline
snapshot mode).

## Built-in rules

The alert engine ships with these rules enabled:

| Rule | Fires when |
|---|---|
| Stale chassis | A chassis stops updating its config (`nb_cfg`) for too long |
| Port down | A port is stuck `up=false` |
| Unbound port | A logical port has no chassis binding |
| BFD down | A BFD session transitions to down |
| Flow-count anomaly | `Logical_Flow` count spikes beyond a threshold |
| HA failover | A gateway chassis failover is detected |

## Inspect alerts over the API

```bash
curl -s http://localhost:8080/api/v1/alerts          # currently firing alerts
curl -s http://localhost:8080/api/v1/alerts/rules    # configured rules
```

## Silence an alert

Create and remove silences:

```bash
curl -s -X POST   http://localhost:8080/api/v1/alerts/silences --data @silence.json
curl -s           http://localhost:8080/api/v1/alerts/silences
curl -s -X DELETE http://localhost:8080/api/v1/alerts/silences/<id>
```

The silence body fields are in the live OpenAPI spec at
<http://localhost:8080/api/v1/docs>.

## Send alerts to webhooks

Point Northwatch at one or more webhook URLs (comma-separated) to receive
notifications when alerts fire:

```bash
./bin/northwatch \
  --ovn-nb-addr tcp:10.0.0.1:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642 \
  --alert-webhook-urls https://hooks.example.com/ovn,https://hooks.example.com/oncall
```

On startup the server prints
`alert webhook notifications enabled (N endpoints)` per cluster. The same value is
available as `NORTHWATCH_ALERT_WEBHOOK_URLS`.

## See it in the lab

The lab's `make lab-sim` triggers real alerts: unbinding a VIF fires "unbound
port", and the gateway-failover simulation exercises the HA-failover path. See
[Run the local lab](/how-to/run-the-local-lab).
