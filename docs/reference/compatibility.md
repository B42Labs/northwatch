# Supported versions

Northwatch supports **OVN 24.03 and newer**. The opt-in per-chassis OVS view
supports **Open vSwitch 3.3 and newer**. One binary covers the whole range; there
is no per-version build.

## Version matrix

| Database | Minimum release | Minimum schema | Validated up to |
|---|---|---|---|
| OVN Northbound | OVN 24.03 | `OVN_Northbound` 7.3.0 | OVN 26.09 (7.19.0) |
| OVN Southbound | OVN 24.03 | `OVN_Southbound` 20.33.0 | OVN 26.09 (21.13.0) |
| Open vSwitch (per chassis) | Open vSwitch 3.3 | `Open_vSwitch` 8.5.0 | Open vSwitch 3.7 (8.8.0) |

"Validated up to" is the newest release whose schema the test suite checks the
models against (`internal/ovsdb/testdata/schemas/`). Releases in between, and
newer ones, work as long as they only add tables and columns, which is how OVN
and Open vSwitch evolve their schemas.

Some distributions and what they ship:

| Distribution | OVN | Supported |
|---|---|---|
| Ubuntu 24.04 | 24.03 | yes |
| DevStack `stable/2026.2` | 24.03 on Ubuntu 24.04; `branch-24.03` when built from source | yes |
| Debian 13 | 25.03 | yes |
| Debian 12 | 23.03 | no |

Northwatch 0.12.0 and earlier required OVN 24.09 and Open vSwitch 3.4.

## Why there is a minimum

The OVSDB models are Go types generated from a schema at build time. When
Northwatch connects, libovsdb checks them against the schema the server reports:
every modeled table and column must exist on the server with the same type.
Tables and columns the server has but the models lack are accepted.

Northwatch therefore generates its models from the oldest supported release
(`OVN_VERSION` / `OVS_VERSION` in the Makefile). A newer server only adds to that
schema, so it passes the check.

## What a newer server does not show

Tables and columns added after the minimum release are not read. They do not
appear in the UI or the API, even when the server has them. Compared with
Northwatch 0.12.0 this means:

| Database | Not shown |
|---|---|
| OVN Northbound | tables `DHCP_Relay`, `Sample`, `Sample_Collector`, `Sampling_App`; columns `ACL.sample_est`, `ACL.sample_new`, `NAT.match`, `NAT.priority`, `Logical_Router_Port.dhcp_relay` |
| OVN Southbound | columns `Logical_Flow.flow_desc`, `IGMP_Group.protocol` |
| Open vSwitch | columns `Mirror.filter`, `Flow_Sample_Collector_Set.local_group_id` |

No analysis, debug view or alert uses any of them.

## What an older server does

**OVN Northbound / Southbound.** Startup fails. The error names the supported
range and then lists what the server's schema lacks. Against OVN 23.03:

```text
error: cluster "default": connecting to OVN: connecting to NB: connecting to tcp:127.0.0.1:6641:
incompatible OVN schema (Northwatch supports OVN 24.03 and newer, see https://b42labs.github.io/northwatch/reference/compatibility):
failed to connect to tcp:127.0.0.1:6641: database OVN_Northbound validation error (4):
Mapper Error. Object type nb.ACL contains field Tier (int) ovs tag tier: Column does not exist in schema. ...
```

libovsdb reports only the first missing column of each table, so the list can
be shorter than the actual difference.

**Open vSwitch.** The chassis is listed as unreachable and its connection is
retried with backoff, logging the same kind of message with the Open vSwitch
range. The other chassis are not affected, and the chassis connects once it is
upgraded.

## Check a server's schema version

Compare the output with the *Minimum schema* column:

```bash
ovsdb-client get-schema-version tcp:127.0.0.1:6641 OVN_Northbound
ovsdb-client get-schema-version tcp:127.0.0.1:6642 OVN_Southbound
ovsdb-client get-schema-version unix:/var/run/openvswitch/db.sock Open_vSwitch
```

## Snapshots from other builds

A snapshot file captured by a build with a higher minimum, such as 0.12.0, still
loads. Tables this build does not model are skipped with a warning in the log.
Columns it does not model are ignored.

## Changing the supported range

The minimum is the oldest OVN LTS release that is still in use, together with
the Open vSwitch release shipped alongside it. It is raised on purpose, when a
feature needs a newer table or column or when that LTS falls out of use. Raising
it drops support for every older release.

To raise the minimum:

1. Set `OVN_VERSION` / `OVS_VERSION` in the `Makefile`.
2. Run `make schema-download` and `make generate`.
3. Update `MinOVNVersion` / `MinOVSVersion` in `internal/ovsdb/compat.go` and the
   expected schema versions in `TestSchemaFloor`.
4. Update this page and `CHANGELOG.md`.

To validate a new release, replace the matching schema in
`internal/ovsdb/testdata/schemas/` with the new release's schema and run
`make test`.
