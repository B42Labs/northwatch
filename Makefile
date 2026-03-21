.PHONY: build test lint generate schema-download clean vet unquarantine build-ui dev-ui build-all ensure-ui-dist openapi-export

OVN_VERSION := v24.09.0
OVN_SCHEMA_BASE := https://raw.githubusercontent.com/ovn-org/ovn/$(OVN_VERSION)

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

clean:
	rm -rf bin/ ui/frontend/dist/ ui/frontend/node_modules/
