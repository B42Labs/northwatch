# Northwatch

[![CI](https://github.com/B42Labs/northwatch/actions/workflows/ci.yml/badge.svg)](https://github.com/B42Labs/northwatch/actions/workflows/ci.yml)

Real-time analyzer, visualizer, debugger and monitor for [OVN](https://www.ovn.org/).
Northwatch connects directly to the OVN Northbound and Southbound OVSDB databases,
keeps a live in-memory copy of both, and serves a unified web UI and REST API for
browsing, correlating, debugging and monitoring an OVN deployment.

## Features

- **Browse** every Northbound and Southbound table, live, from one place
- **Omnisearch** — start from any IP, MAC, UUID or name and find everything related across both databases
- **Correlate** Northbound intent with Southbound realization (`Logical_Switch_Port` → `Port_Binding` → `Chassis`)
- **Visualize** logical, physical, gateway, NAT and load-balancer topology
- **Debug** — packet trace, connectivity checks, port-binding diagnostics, ACL audit, stale-entry detection, flow diff
- **OVS reality check** (opt-in) — live per-chassis Open_vSwitch state correlated with OVN intent, plus fleet-wide OVS health
- **Enrich** OVN UUIDs into human-readable names via OpenStack or Kubernetes
- **Monitor** — telemetry with a Prometheus endpoint, an alert engine with webhook notifications
- **History** — a SQLite event log and periodic snapshots, with diffing and offline replay
- **Multi-cluster** — watch several OVN deployments from a single process
- **Write** (opt-in) — mutate OVN behind a plan/preview/apply workflow with an audit log
- **Hardened** — bearer-token auth on every mutating route, optional TLS for the API and all OVSDB connections, Debian packaging with a systemd unit

## Quick start

The fastest way to see Northwatch is the bundled OVN lab — a throwaway control
plane plus a load generator that keeps it busy:

```bash
make lab-compose-up    # 1 central + 3 chassis, NB :6641 / SB :6642 (Docker Desktop works)
make lab-seed          # create a realistic baseline topology
make lab-bind          # bind the seeded ports onto chassis

make build             # -> bin/northwatch
./bin/northwatch --ovn-nb-addr tcp:127.0.0.1:6641 --ovn-sb-addr tcp:127.0.0.1:6642
# open http://localhost:8080
```

Point it at your own deployment by passing its Northbound and Southbound
addresses (comma-separated for Raft failover):

```bash
./bin/northwatch \
  --ovn-nb-addr tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642
```

See [Getting started](https://b42labs.github.io/northwatch/tutorials/getting-started)
for the full walkthrough.

## Documentation

Full documentation lives at **<https://b42labs.github.io/northwatch/>**,
organized along the [Diátaxis](https://diataxis.fr/) framework:

- [Tutorials](https://b42labs.github.io/northwatch/tutorials/) — learning-oriented, guided paths
- [How-to guides](https://b42labs.github.io/northwatch/how-to/) — task-oriented recipes
- [Reference](https://b42labs.github.io/northwatch/reference/) — every CLI flag, config field, route and metric
- [Explanation](https://b42labs.github.io/northwatch/explanation/) — architecture, correlation model, and design decisions

The docs are built with [VitePress](https://vitepress.dev/) and live in
[`docs/`](docs/). To work on them locally:

```bash
cd docs && npm install && npm run docs:dev
```

## License

Licensed under the Apache License 2.0 — see [LICENSE](LICENSE).
