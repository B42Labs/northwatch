# Enrich with OpenStack

OpenStack is the primary enrichment provider. With it configured, Northwatch
resolves OVN UUIDs and `external_ids` to human-readable OpenStack names —
networks, ports, instances, routers and floating IPs — so a chassis or port reads
as `web-server-01 on project production` instead of a bare UUID.

Enrichment is always additive: the core works without it, and turning it on only
adds context.

## Configure with flags

Setting `--os-auth-url` enables the OpenStack provider for the default cluster.
The standard `OS_*` environment variables are read as defaults, so an existing
OpenStack RC file mostly works as-is:

```bash
./bin/northwatch \
  --ovn-nb-addr tcp:10.0.0.1:6641 \
  --ovn-sb-addr tcp:10.0.0.1:6642 \
  --os-auth-url https://keystone.example.com:5000/v3 \
  --os-username northwatch \
  --os-password secret \
  --os-project-name admin \
  --os-domain-name Default \
  --os-region-name RegionOne
```

| Flag | Env var |
|---|---|
| `--os-auth-url` | `OS_AUTH_URL` |
| `--os-username` | `OS_USERNAME` |
| `--os-password` | `OS_PASSWORD` |
| `--os-project-name` | `OS_PROJECT_NAME` |
| `--os-domain-name` | `OS_USER_DOMAIN_NAME` |
| `--os-region-name` | `OS_REGION_NAME` |

## Configure per cluster

In a multi-cluster setup, put the credentials in the cluster's `enrichment`
block instead (see [Monitor multiple clusters](/how-to/monitor-multiple-clusters)):

```json
{
  "name": "production",
  "ovn_nb_addr": "tcp:10.0.0.1:6641",
  "ovn_sb_addr": "tcp:10.0.0.1:6642",
  "enrichment": {
    "type": "openstack",
    "os_auth_url": "https://keystone.example.com:5000/v3",
    "os_username": "northwatch",
    "os_password": "secret",
    "os_project_name": "admin",
    "os_domain_name": "Default",
    "os_region_name": "RegionOne"
  }
}
```

## What gets resolved

| OVN entity | `external_ids` / key | Resolved to |
|---|---|---|
| `Logical_Switch` | `neutron:network_name` | Neutron network name |
| `Logical_Switch_Port` | `neutron:port_name`, `neutron:device_id` | Port name, Nova instance |
| `Logical_Router` | `neutron:router_name` | Neutron router name |
| `NAT` (floating IP) | `neutron:fip_id` | Floating-IP details |
| `Port_Binding` (SB) | `iface-id` | Port / instance info |
| `Chassis` | hostname | Nova hypervisor details |

## Tune the cache

Enrichment results are cached. Control the refresh interval with
`--enrichment-cache-ttl` (default `5m`):

```bash
./bin/northwatch ... --enrichment-cache-ttl 10m
```

## Confirm it is on

When enrichment is configured, the server advertises an `enrich` capability:

```bash
curl -s http://localhost:8080/api/v1/capabilities
```

The `correlated/*` endpoints then include the resolved names inline. For the
design, see [Enrichment](/explanation/enrichment).
