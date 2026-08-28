# Enrichment

OVN identifies everything by UUID. A `Port_Binding` for
`a1b2c3d4-…` is correct but unreadable; what an operator wants to see is
`web-server-01 on project production`. Enrichment is the layer that resolves
OVN identifiers to human-readable names from the system that created them.

## Always additive

The core of Northwatch works with no enrichment provider configured. You still
get every table, every correlation, search and tracing, just with raw UUIDs and
`external_ids`. Enrichment only adds names. This keeps the tool useful against
any OVN deployment, and means a misconfigured or unreachable provider degrades to
"no names" rather than breaking browsing.

When a provider is configured, the server advertises the `enrich` capability and
the `correlated/*` views include the resolved context inline.

## Providers

Enrichment is pluggable; a provider knows how to map OVN entities to one external
system:

- OpenStack, the primary provider, resolves through Neutron, Nova and
  Keystone: switches to networks, ports to Neutron ports and Nova instances,
  routers to Neutron routers, NAT entries to floating-IP details, and project
  IDs to project names. The mapping keys off the `neutron:*` `external_ids` that
  Neutron writes onto OVN switches, ports, routers and NAT entries. See
  [Enrich with OpenStack](/how-to/enrich-with-openstack).
- Kubernetes resolves ovn-kubernetes entities to namespaces, pods and nodes
  via a kubeconfig. See [Enrich with Kubernetes](/how-to/enrich-with-kubernetes).

Each cluster uses at most one provider, configured either by the flat OpenStack /
Kubernetes flags (default cluster) or per-cluster in the
[config file](/reference/configuration).

## Caching strategy

External APIs are slow and rate-limited compared to a local cache, so enrichment
results are cached with a TTL (`--enrichment-cache-ttl`, default `5m`). The design
goal is that enrichment never blocks a read:

- A cache hit returns the name immediately.
- A cache miss returns the raw identifier now and resolves the name in the
  background; the UI fills it in when it arrives.

This way the OVN data, which is already local and fast, is never held up waiting
on Neutron or the Kubernetes API. Enrichment is an overlay on top of OVN truth,
not a dependency of it.

## Related

- [Enrich with OpenStack](/how-to/enrich-with-openstack)
- [Enrich with Kubernetes](/how-to/enrich-with-kubernetes)
- [Correlation & search](/explanation/correlation-and-search)
