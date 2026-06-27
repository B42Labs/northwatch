# Configuration file

For multi-cluster setups, Northwatch reads a JSON config file given with
`--config-file` (or `NORTHWATCH_CONFIG_FILE`). It defines one or more clusters,
each with its own OVN addresses and optional enrichment. A single cluster can be
configured this way too, but the flat `--ovn-nb-addr` / `--ovn-sb-addr` flags are
simpler for that case.

## Schema

```json
{
  "clusters": [
    {
      "name": "production",
      "label": "Production",
      "ovn_nb_addr": "tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641",
      "ovn_sb_addr": "tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642",
      "enrichment": {
        "type": "openstack",
        "os_auth_url": "https://keystone.example.com:5000/v3",
        "os_username": "northwatch",
        "os_password": "secret",
        "os_project_name": "admin",
        "os_domain_name": "Default",
        "os_region_name": "RegionOne",
        "os_cacert": "/etc/northwatch/testbed-ca.pem"
      }
    }
  ]
}
```

### Top level

| Field | Type | Required | Description |
|---|---|---|---|
| `clusters` | array | yes | One or more cluster definitions. Must contain at least one. |

### Cluster object

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Unique identifier. Used in the `/api/v1/clusters/{name}/...` routes. |
| `label` | string | no | Display label. Defaults to `name`. |
| `ovn_nb_addr` | string | yes | Northbound address; comma-separated for Raft failover. |
| `ovn_sb_addr` | string | yes | Southbound address; comma-separated for Raft failover. |
| `enrichment` | object | no | Per-cluster enrichment provider. Omit for none. |

### Enrichment object

`type` selects the provider; the remaining fields depend on it.

| Field | Type | Applies to | Description |
|---|---|---|---|
| `type` | string | — | `openstack` or `kubernetes`. |
| `os_auth_url` | string | openstack | Keystone auth URL. |
| `os_username` | string | openstack | Username. |
| `os_password` | string | openstack | Password. |
| `os_project_name` | string | openstack | Project name. |
| `os_domain_name` | string | openstack | User domain name. |
| `os_region_name` | string | openstack | Region name. |
| `os_cacert` | string | openstack | Path to a PEM CA bundle for verifying the OpenStack API (clouds.yaml `cacert`). |
| `kubeconfig` | string | kubernetes | Path to kubeconfig. |
| `kube_context` | string | kubernetes | Kubeconfig context. |

A cluster has at most one enrichment provider.

## Behaviour

- The **first** cluster in the list is the **default** — it is served at the
  top-level `/api/v1/...` routes, and its history, snapshots and Prometheus
  metrics are the ones exposed globally.
- Every cluster, including the default, is additionally reachable under
  `/api/v1/clusters/{name}/...`.
- The enrichment cache TTL is global, set with `--enrichment-cache-ttl`, not in
  the file.
- The chassis-inventory staleness threshold is global, set with
  `--chassis-stale-threshold`, not in the file. It controls when a chassis is
  reported not-alive; because `nb_cfg_timestamp` only advances on a new config
  generation, low-churn deployments should raise it.

## Precedence

1. `--snapshot` — if set, serve the file offline; ignore everything else.
2. `--config-file` — if set, use the file; ignore the flat NB/SB flags.
3. Flat `--ovn-nb-addr` / `--ovn-sb-addr` — used otherwise; both required.

## Validation errors

The file is rejected at startup if:

- it defines zero clusters,
- a cluster is missing `name`, `ovn_nb_addr`, or `ovn_sb_addr`.

## Related

- [Monitor multiple clusters](/how-to/monitor-multiple-clusters)
- [CLI flags & environment variables](/reference/cli)
