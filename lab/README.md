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

- The lab uses the **OVS userspace datapath** (`datapath_type=netdev`): it only
  needs OVN/OVS *control-plane* state, never real packet forwarding, so there is
  no host kernel-module or FRR/BGP dependency. Traffic between chassis does not
  actually flow — that is fine for an observability lab.
- Two ways to run it:
  - **Docker Compose** (`docker-compose.yml`) — works directly on Docker /
    **Docker Desktop**, including macOS. No extra tooling. *Recommended on macOS.*
  - **containerlab** (`topology.clab.yml`) — needs a **Linux Docker host**; on
    macOS run it inside a Linux VM (Colima / OrbStack / Lima).

## macOS / Docker Desktop (Compose)

containerlab is a Linux-only tool, so on macOS the simplest path is the Compose
variant — the topology has no special point-to-point links, so plain Compose is
enough:

```sh
make lab-compose-up    # build images + start (1 central + 3 chassis), NB/SB on :6641/:6642
make lab-seed          # seed the baseline topology (runs on the host)
make lab-bind          # bind the seeded VIFs onto chassis (clears "unbound VIF" alerts)
make build && ./bin/northwatch --ovn-nb-addr tcp:127.0.0.1:6641 --ovn-sb-addr tcp:127.0.0.1:6642
make lab-sim           # continuous change (Ctrl-C to stop)
make lab-compose-down  # tear down
```

`make lab-compose` is a shortcut that brings the lab up, waits for OVN, and
seeds it. The containers are named `clab-nw-lab-*`, so `make lab-nbctl`,
`make lab-sbctl` and `ovnsim --bind-ports` work the same as with containerlab.

> On Apple Silicon the images build natively for arm64 (OVN/OVS come from the
> Ubuntu Cloud Archive, which ships both arches). The chassis containers run
> `privileged` so OVS can manage its bridges inside the Docker VM.

## Quick start (containerlab, Linux)

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
  with NAT / static routes / policies, gateway-port redundancy via both
  `Gateway_Chassis` (odd routers) and `HA_Chassis_Group` (even routers), a
  security port group with ACLs + address sets, load balancers, meters, DNS,
  etc. `ovn-northd` then computes the Southbound state (Logical_Flow,
  Datapath_Binding, …) automatically.
- `ovnsim run` — once per `--interval`, perform one weighted-random change:
  create/delete switches and routers, add/remove ports, flip NAT/ACL/LB rules,
  toggle port admin state, **simulate gateway failover** by swapping
  HA_Chassis_Group member priorities, and add/remove HA group members. The mix
  is biased toward a target switch count so the object set stays lively but
  bounded. With `--bind-ports` it also creates real OVS interfaces on the
  chassis (`docker exec … ovs-vsctl`) and migrates them between chassis, so
  `Port_Binding.chassis` actually moves in the dashboard.
- `ovnsim bind` / `ovnsim unbind` — bind every seeded VIF onto a chassis
  round-robin (creating real OVS interfaces), or remove those bindings. Use
  `bind` right after `seed` to make the baseline look like a fully-running
  deployment — see the note on the "VIF not bound" alert below.
- `ovnsim clean` — delete everything `ovnsim` created.

> **"VIF port not bound to any chassis" health alerts after seeding are
> expected.** `seed` only creates the *logical* NB ports; with no backing OVS
> interface on any chassis, `ovn-controller` never binds them, so `Port_Binding`
> has no chassis and Northwatch correctly flags them. Clear them by binding the
> ports onto chassis: run `make lab-bind` once, or `make lab-sim` — `run
> --bind-ports` binds every existing unbound VIF on startup and binds each new
> port it creates, so the lab stays healthy. (`make lab-unbind` reverses it, to
> demo the alert on purpose.)

> **The HA_Chassis_Group gateway showing "no active chassis" (no-owner) is
> expected here.** ovn-controller elects the active chassis for a multi-member
> HA group via BFD between the chassis, and BFD never converges over the OVS
> *userspace* datapath (no real inter-chassis tunnel traffic) — the same
> limitation the upstream e2e lab documents (it uses the kernel datapath, which
> isn't available on Docker Desktop). So the multi-member HA gateway stays
> unbound and Northwatch correctly flags it; this actually demonstrates the tool
> catching a stuck failover. The single-candidate gateways (`Gateway_Chassis`,
> odd routers) bind fine. The failover *simulation* still works regardless: the
> `ha.failover` action swaps member priorities and the gateway view's "desired"
> chassis moves accordingly.

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
