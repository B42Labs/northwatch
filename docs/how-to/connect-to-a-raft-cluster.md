# Connect to a Raft cluster

In production, the OVN databases usually run as a 3- or 5-node clustered (Raft)
deployment. Northwatch connects to one endpoint at a time and fails over to the
next when a connection drops.

## Pass multiple endpoints

Give `--ovn-nb-addr` and `--ovn-sb-addr` a comma-separated list of endpoints, one
per cluster member:

```bash
./bin/northwatch \
  --ovn-nb-addr tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642
```

Northwatch connects to a reachable endpoint, monitors both databases, and
re-connects against the remaining endpoints if that member becomes unavailable.

The same values can be supplied as environment variables:

```bash
export NORTHWATCH_OVN_NB_ADDR=tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641
export NORTHWATCH_OVN_SB_ADDR=tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642
./bin/northwatch
```

## Watch cluster health

Northwatch tracks per-member Raft state through the `_Server` database and
exposes it:

```bash
curl -s http://localhost:8080/api/v1/telemetry/raft-health
```

The same data feeds the Prometheus metrics `northwatch_ovsdb_connected` and
`northwatch_ovsdb_connections` — see [Prometheus metrics](/reference/metrics).

## Reduce load on the primary

The initial monitor is the heaviest moment for the database serving it. To keep
that load off the Raft leader, point Northwatch at an **OVSDB relay** or a
**standby/follower** endpoint, and tune the staged monitor for large
deployments. See [Tune the initial load](/how-to/tune-the-initial-load).

## Next steps

- Monitor several independent clusters at once:
  [Monitor multiple clusters](/how-to/monitor-multiple-clusters).
- Full flag list: [CLI flags & env vars](/reference/cli).
