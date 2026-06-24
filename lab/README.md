# Northwatch local OVN lab

A throwaway OVN deployment for developing and demoing Northwatch. It brings up a
real OVN control plane — `ovn-northd` + NB/SB databases on a `central` node and
three `chassis` nodes running Open vSwitch + `ovn-controller` — and a load
generator (`ovnsim`) that fills it with realistic objects and keeps mutating
them, so the dashboard, history, events and alerts all have something to show.

```
            ┌─────────────────────────────┐
            │  central                    │   NB :6641  ─┐  (published to host)
            │  ovn-northd + NB/SB ovsdb   │   SB :6642  ─┤
            └─────────────────────────────┘             │
              ▲          ▲          ▲                    ▼
       ┌──────────┐ ┌──────────┐ ┌──────────┐    ┌──────────────┐
       │ chassis-1│ │ chassis-2│ │ chassis-3│    │  Northwatch  │  (runs on host)
       │ OVS+ctrl │ │ OVS+ctrl │ │ OVS+ctrl │    │  :8080       │
       └──────────┘ └──────────┘ └──────────┘    └──────────────┘
                         ▲
                         │ NB writes + (optional) ovs-vsctl bindings
                  ┌──────────────┐
                  │   ovnsim     │  (runs on host:  make lab-seed / lab-sim)
                  └──────────────┘
```

## Requirements

- A **Linux Docker host** plus [containerlab](https://containerlab.dev).
  - On **macOS**, run the lab inside a Linux VM (Colima / OrbStack / Lima);
    containerlab needs Linux. `make lab-install-tools` prints the details.
- The lab uses the **OVS userspace datapath** (`datapath_type=netdev`): it only
  needs OVN/OVS *control-plane* state, never real packet forwarding, so there is
  no host kernel-module or FRR/BGP dependency. Traffic between chassis does not
  actually flow — that is fine for an observability lab.

## Quick start

```sh
make lab-install-tools   # one-time: install containerlab (Linux)
make lab-up              # build images + deploy the topology (1 central + 3 chassis)
make lab-seed            # create the baseline OVN objects (the dashboard "Grundlast")

# Start Northwatch on the host against the lab:
make build && ./bin/northwatch --ovn-nb-addr tcp:127.0.0.1:6641 --ovn-sb-addr tcp:127.0.0.1:6642
# open http://localhost:8080

make lab-sim             # in another terminal: continuous change (Ctrl-C to stop)
make lab-down            # tear everything down
```

`make lab` is a shortcut for `lab-up` + `lab-seed` that then prints the
Northwatch command to run.

## What `ovnsim` does

`ovnsim` (in `cmd/ovnsim`, logic in `internal/ovnsim`) writes directly to OVN
Northbound over OVSDB using the project's generated NB models.

- `ovnsim seed` — create an idempotent baseline that touches every major NB
  table: tenant switches with VIF ports + DHCP, routers wired to those switches
  with NAT / static routes / policies, a security port group with ACLs + address
  sets, load balancers, meters, DNS, etc. `ovn-northd` then computes the
  Southbound state (Logical_Flow, Datapath_Binding, …) automatically.
- `ovnsim run` — once per `--interval`, perform one weighted-random change:
  create/delete switches and routers, add/remove ports, flip NAT/ACL/LB rules,
  toggle port admin state. The mix is biased toward a target switch count so the
  object set stays lively but bounded. With `--bind-ports` it also creates real
  OVS interfaces on the chassis (`docker exec … ovs-vsctl`) and migrates them
  between chassis, so `Port_Binding.chassis` actually moves in the dashboard.
- `ovnsim clean` — delete everything `ovnsim` created.

Every object `ovnsim` creates is tagged (`external_ids:nw-sim="1"`) and named
`nw-…`, and `run`/`clean` only ever touch those rows — so pointing it at a
database that already has other content is safe.

Useful while the lab is up:

```sh
make lab-nbctl ARGS=show
make lab-sbctl ARGS="list Chassis"
make lab-sbctl ARGS="list Logical_Flow"
```

## Multi-cluster (optional)

`make lab-multi-up` deploys a second, independent cluster (`nw-lab2`,
NB `:6643` / SB `:6644`) to exercise Northwatch's multi-cluster view. Point
Northwatch at both with a `--config-file` listing two clusters, then seed each
(`make lab-seed NB=tcp:127.0.0.1:6643`). `make lab-multi-down` removes it.

## Files

| File | Purpose |
|------|---------|
| `topology.clab.yml` | default single-cluster topology (1 central + 3 chassis) |
| `topology-multi.clab.yml` | optional second cluster |
| `Dockerfile.central` / `central-entrypoint.sh` | ovn-northd + NB/SB ovsdb-server |
| `Dockerfile.chassis` / `chassis-entrypoint.sh` | OVS (userspace) + ovn-controller |
