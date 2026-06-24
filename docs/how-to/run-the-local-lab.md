# Run the local lab

The repository ships a throwaway OVN deployment under `lab/` for development and
demos. It brings up a real control plane — `ovn-northd` plus Northbound/Southbound
databases on a `central` node and three `chassis` nodes running Open vSwitch and
`ovn-controller` — and a load generator (`ovnsim`) that fills it with realistic
objects and keeps mutating them, so the dashboard, history, events and alerts all
have something to show.

The lab uses the OVS **userspace** datapath: it only needs control-plane state,
never real packet forwarding, so there is no kernel-module or BGP dependency.

## Option A: Docker Compose (recommended on macOS)

Compose works on plain Docker and Docker Desktop, with no extra tooling:

```bash
make lab-compose-up    # build images + start 1 central + 3 chassis (NB :6641 / SB :6642)
make lab-seed          # seed the baseline topology (runs on the host)
make lab-bind          # bind the seeded VIFs onto chassis (clears "unbound VIF" alerts)
make lab-sim           # continuous change (Ctrl-C to stop)
make lab-compose-down  # tear down
```

`make lab-compose` is a shortcut that brings the lab up, waits for OVN to be
ready, and seeds it in one step.

## Option B: containerlab (Linux)

On a Linux Docker host you can use [containerlab](https://containerlab.dev):

```bash
make lab-install-tools   # one-time: install containerlab
make lab-up              # build images + deploy the topology
make lab-seed
make lab-down
```

`make lab` is a shortcut for `lab-up` + `lab-seed`.

## Point Northwatch at the lab

Both options publish the databases on the host at the same ports:

```bash
make build
./bin/northwatch --ovn-nb-addr tcp:127.0.0.1:6641 --ovn-sb-addr tcp:127.0.0.1:6642
```

Then open <http://localhost:8080>.

## Drive change with `ovnsim`

| Command | What it does |
|---|---|
| `make lab-seed` | Create an idempotent baseline across every major NB table. |
| `make lab-bind` | Bind every seeded VIF onto a chassis (creates real OVS interfaces). |
| `make lab-unbind` | Remove those bindings — useful to demo the "unbound VIF" alert. |
| `make lab-sim` | Continuously mutate the topology (foreground; Ctrl-C to stop). |
| `make lab-reseed` | Clean, re-seed and re-bind for a fresh baseline. |
| `make lab-clean` | Remove everything `ovnsim` created (leaves containers running). |

Everything `ovnsim` creates is tagged `external_ids:nw-sim="1"` and named `nw-…`,
and `run`/`clean` only touch those rows, so pointing it at a database that
already has content is safe.

## Inspect the lab directly

```bash
make lab-nbctl ARGS=show
make lab-sbctl ARGS="list Chassis"
```

## Expected alerts

Two health alerts are **expected** in the userspace-datapath lab:

- **"VIF port not bound to any chassis"** after seeding — `seed` creates only the
  logical ports; run `make lab-bind` (or `make lab-sim`) to bind them.
- **HA gateway "no active chassis" (no-owner)** for multi-member HA groups — BFD
  never converges over the userspace datapath, so the active chassis is never
  elected. To make multi-member HA gateways bind for real, run on the kernel
  datapath: `make lab-compose-up KERNEL=1` (requires a Linux host whose kernel
  ships the `openvswitch` module).

Both demonstrate Northwatch correctly catching an incomplete realization.

## A second cluster

`make lab-multi-up` deploys an independent second cluster (NB `:6643` / SB
`:6644`) for the multi-cluster view; `make lab-multi-down` removes it. See
[Monitor multiple clusters](/how-to/monitor-multiple-clusters).
