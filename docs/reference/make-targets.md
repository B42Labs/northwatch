# Make targets

The `Makefile` is the entry point for building, testing and running the local
lab. This page lists the targets; the lab targets are covered in detail in
[Run the local lab](/how-to/run-the-local-lab).

## Build & test

| Target | What it does |
|---|---|
| `make build` | Build the binary to `bin/northwatch` (ensures a UI dist exists first). |
| `make build-ui` | Build the Svelte frontend (`npm ci && npm run build`). |
| `make build-all` | `build-ui` then `build`. |
| `make dev-ui` | Run the frontend dev server. |
| `make test` | Run all Go tests with the race detector (`go test -race ./...`). |
| `make lint` | Run `golangci-lint`. |
| `make vet` | Run `go vet ./...`. |
| `make clean` | Remove `bin/` and frontend build artifacts. |

## Code generation

| Target | What it does |
|---|---|
| `make generate` | Regenerate the OVSDB models from the schemas. |
| `make schema-download` | Download the pinned OVN NB/SB schemas (`OVN_VERSION` in the Makefile). |
| `make openapi-export` | Write the OpenAPI spec to `openapi.json`. |
| `make ovnsim` | Build the `ovnsim` load generator to `bin/ovnsim`. |

## macOS note

`make unquarantine` strips the macOS quarantine attribute from `bin/northwatch*`
so a freshly built binary runs without a Gatekeeper prompt.

## Lab targets

| Target | What it does |
|---|---|
| `make lab-compose-up` / `lab-compose-down` | Start / stop the Docker Compose lab. |
| `make lab-compose` | Compose up, wait for OVN, then seed. |
| `make lab-up` / `lab-down` | Start / stop the containerlab lab (Linux). |
| `make lab` | `lab-up` + `lab-seed`. |
| `make lab-seed` | Create the baseline OVN topology. |
| `make lab-bind` / `lab-unbind` | Bind / unbind seeded VIFs onto chassis. |
| `make lab-reseed` | `lab-clean` + `lab-seed` + `lab-bind`. |
| `make lab-sim` | Continuously mutate the topology (foreground). |
| `make lab-clean` | Remove everything `ovnsim` created. |
| `make lab-nbctl ARGS=...` / `lab-sbctl ARGS=...` | Run `ovn-nbctl` / `ovn-sbctl` in the central container. |
| `make lab-multi-up` / `lab-multi-down` | Start / stop a second independent cluster. |
| `make lab-images` | Build the central and chassis images. |
| `make lab-install-tools` | Install containerlab (Linux). |

## Useful variables

| Variable | Default | Purpose |
|---|---|---|
| `NB` | `tcp:127.0.0.1:6641` | Northbound address used by lab targets. |
| `SB` | `tcp:127.0.0.1:6642` | Southbound address used by lab targets. |
| `LAB_NAME` | `nw-lab` | Lab name (container name prefix). |
| `KERNEL` | *(unset)* | `KERNEL=1` layers in the kernel-datapath Compose override for real HA failover. |

```bash
# Re-seed a second lab cluster:
make lab-seed NB=tcp:127.0.0.1:6643

# Real HA failover on a Linux host:
make lab-compose-up KERNEL=1
```
