.PHONY: build test lint generate schema-download clean vet unquarantine build-ui dev-ui build-all ensure-ui-dist openapi-export \
	ovnsim lab-images lab-up lab-down lab lab-seed lab-sim lab-clean lab-nbctl lab-sbctl lab-install-tools lab-multi-up lab-multi-down

OVN_VERSION := v24.09.0
OVN_SCHEMA_BASE := https://raw.githubusercontent.com/ovn-org/ovn/$(OVN_VERSION)

# --- Local OVN lab (containerlab) -------------------------------------------
# A throwaway OVN deployment for developing and demoing Northwatch. See lab/.
# Requires a Linux Docker host (on macOS run via Colima/OrbStack/Lima).
LAB_NAME    ?= nw-lab
LAB_TOPO    ?= lab/topology.clab.yml
LAB_MULTI   ?= lab/topology-multi.clab.yml
NB          ?= tcp:127.0.0.1:6641
SB          ?= tcp:127.0.0.1:6642
CENTRAL     := clab-$(LAB_NAME)-central

build: ensure-ui-dist
	go build -o bin/northwatch ./cmd/northwatch/

build-ui:
	cd ui/frontend && npm ci && npm run build

dev-ui:
	cd ui/frontend && npm run dev

build-all: build-ui build

ensure-ui-dist:
	@mkdir -p ui/frontend/dist
	@test -f ui/frontend/dist/index.html || echo '<!doctype html><html><body>Run make build-ui</body></html>' > ui/frontend/dist/index.html

test:
	go test -race ./...

lint:
	golangci-lint run

vet:
	go vet ./...

generate:
	go generate ./internal/ovsdb/...

schema-download:
	curl -sL $(OVN_SCHEMA_BASE)/ovn-nb.ovsschema -o internal/ovsdb/nb/ovn-nb.ovsschema
	curl -sL $(OVN_SCHEMA_BASE)/ovn-sb.ovsschema -o internal/ovsdb/sb/ovn-sb.ovsschema

unquarantine:
	xattr -d com.apple.quarantine bin/northwatch*

openapi-export:
	go run ./cmd/openapi-export > openapi.json

ovnsim:
	go build -o bin/ovnsim ./cmd/ovnsim/

# Build the central (ovn-northd + NB/SB) and chassis (OVS + ovn-controller) images.
lab-images:
	docker build -f lab/Dockerfile.central -t northwatch/lab-central:latest lab
	docker build -f lab/Dockerfile.chassis -t northwatch/lab-chassis:latest lab

# Bring up the single-cluster lab (1 central + 3 chassis) and publish NB/SB on the host.
lab-up: lab-images
	containerlab deploy -t $(LAB_TOPO)

# Tear the lab down.
lab-down:
	containerlab destroy -t $(LAB_TOPO)

# One-shot: bring the lab up and seed a baseline topology, then print next steps.
lab: lab-up lab-seed
	@echo ""
	@echo "Lab is up and seeded. Start Northwatch against it with:"
	@echo "  make build && ./bin/northwatch --ovn-nb-addr $(NB) --ovn-sb-addr $(SB)"
	@echo "Then open http://localhost:8080 and run 'make lab-sim' for continuous change."

# Create the baseline OVN objects (the dashboard "Grundlast").
lab-seed:
	go run ./cmd/ovnsim seed --nb $(NB)

# Continuously mutate the topology; --bind-ports also binds ports onto chassis.
# Runs in the foreground — Ctrl-C to stop.
lab-sim:
	go run ./cmd/ovnsim run --nb $(NB) --bind-ports --lab-name $(LAB_NAME)

# Remove everything ovnsim created (leaves the lab containers running).
lab-clean:
	go run ./cmd/ovnsim clean --nb $(NB)

# Convenience wrappers: `make lab-nbctl ARGS=show`, `make lab-sbctl ARGS="list Chassis"`.
lab-nbctl:
	docker exec $(CENTRAL) ovn-nbctl $(ARGS)

lab-sbctl:
	docker exec $(CENTRAL) ovn-sbctl $(ARGS)

# Optional second, independent cluster (NB 6643 / SB 6644) for the multi-cluster view.
lab-multi-up: lab-images
	containerlab deploy -t $(LAB_MULTI)

lab-multi-down:
	containerlab destroy -t $(LAB_MULTI)

# Install containerlab. On macOS, run the lab inside a Linux VM (Colima/OrbStack/Lima);
# containerlab itself needs a Linux Docker host.
lab-install-tools:
	@echo "Installing containerlab (Linux)..."
	@echo "  bash -c \"$$(curl -sL https://get.containerlab.dev)\""
	@echo "On macOS: run the lab in a Linux VM (e.g. 'colima start' / OrbStack) — containerlab needs Linux."
	bash -c "$$(curl -sL https://get.containerlab.dev)"

clean:
	rm -rf bin/ ui/frontend/dist/ ui/frontend/node_modules/
