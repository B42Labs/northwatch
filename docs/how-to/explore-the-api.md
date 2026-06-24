# Explore the API

The Northwatch HTTP API is documented by an OpenAPI 3.1 specification that the
server generates and serves itself. That spec — not this site — is the exhaustive,
always-current description of every route, parameter and response.

## Swagger UI

Open the interactive documentation in a browser:

```
http://localhost:8080/api/v1/docs
```

From there you can read every endpoint and call it directly against the running
server.

## Raw OpenAPI spec

Fetch the machine-readable spec for tooling or client generation:

```bash
curl -s http://localhost:8080/api/v1/openapi.json
```

## Export the spec without a running server

The `openapi-export` helper writes the spec to stdout, which is handy in CI or for
generating clients at build time:

```bash
make openapi-export      # writes openapi.json
# or:
go run ./cmd/openapi-export > openapi.json
```

## Quick orientation

All API routes are under `/api/v1`. Responses are JSON. List endpoints return a
JSON array of objects keyed by their OVSDB column names; detail endpoints return a
single object; errors return `{"error": "..."}` with the matching HTTP status.

For the route groups and the response shape, see [HTTP API](/reference/api). To
generate a typed client, point your OpenAPI generator at the `openapi.json` above.
