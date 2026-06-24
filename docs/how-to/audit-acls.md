# Audit ACLs

OVN ACLs are ordered by priority, and a higher-priority rule that already matches
all traffic makes a lower-priority rule unreachable. Northwatch's ACL audit finds
these shadowed and conflicting rules so you can spot dead security policy.

The audit is part of the `debug` capability, on by default.

## Run the audit

```bash
curl -s http://localhost:8080/api/v1/debug/acl-audit
```

The result enumerates ACLs with their priority, direction, match and action, and
flags rules that can never match because a higher-priority rule shadows them.

## Browse the raw ACL state

To inspect ACLs, port groups and address sets directly:

```bash
curl -s http://localhost:8080/api/v1/nb/acls
curl -s http://localhost:8080/api/v1/nb/port-groups
curl -s http://localhost:8080/api/v1/nb/address-sets
```

With OpenStack enrichment on, a port group maps to a security group and an
address set maps to a security-group remote reference, so the audit reads in
terms of your tenant's security groups. See
[Enrich with OpenStack](/how-to/enrich-with-openstack).

## Search for a specific rule

To find every ACL whose match references an IP or address set, use Omnisearch:

```bash
curl -s 'http://localhost:8080/api/v1/search?q=10.0.0.0/24'
```

## Related

- [Search with Omnisearch](/how-to/search-with-omnisearch)
- [Capabilities](/reference/capabilities)
