# Monitor multiple clusters

A single Northwatch process can monitor several independent OVN deployments at
once. You describe them in a JSON config file and pass it with `--config-file`.

## Write a config file

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
        "os_region_name": "RegionOne"
      }
    },
    {
      "name": "staging",
      "label": "Staging",
      "ovn_nb_addr": "tcp:ovn-nb.staging.example.com:6641",
      "ovn_sb_addr": "tcp:ovn-sb.staging.example.com:6642"
    }
  ]
}
```

Each cluster needs a unique `name` and both addresses; `label` defaults to the
name, and `enrichment` is optional and per-cluster. For the full field list see
[Configuration file](/reference/configuration).

## Start with the config file

```bash
./bin/northwatch --config-file /etc/northwatch/clusters.json
```

When the file defines more than one cluster, Northwatch logs
`multi-cluster mode enabled` with the cluster count and advertises the
`multi-cluster` capability.

::: warning Precedence
`--snapshot` takes precedence over everything. Otherwise, when `--config-file` is
set, the flat `--ovn-nb-addr` / `--ovn-sb-addr` flags are ignored.
:::

## Reach each cluster

The first cluster in the file is the **default** and is served at the top-level
routes (`/api/v1/...`). Every cluster — including the default — is also reachable
under a per-cluster prefix:

```bash
curl -s http://localhost:8080/api/v1/clusters                      # list clusters
curl -s http://localhost:8080/api/v1/clusters/staging/topology     # staging's topology
curl -s http://localhost:8080/api/v1/clusters/production/search?q=10.0.0.42
```

## Try it with the lab

Bring up the bundled second lab cluster and point a config file at both:

```bash
make lab-multi-up        # second cluster on NB :6643 / SB :6644
make lab-seed NB=tcp:127.0.0.1:6643
```

Then list two clusters in your config file (`tcp:127.0.0.1:6641/6642` and
`tcp:127.0.0.1:6643/6644`) and start Northwatch with `--config-file`.

## Next steps

- [Configuration file](/reference/configuration) — every field.
- [Enrich with OpenStack](/how-to/enrich-with-openstack) — per-cluster enrichment.
