# contrib

Supporting files for developing against and demonstrating Northwatch. These are
lab/development aids, not runtime components of the server.

## `testbed.pem`

`testbed.pem` is the **public "OSISM Testbed CA" X.509 certificate** — a trust
anchor only, with **no private key**. It is safe to commit and to share.

It is used solely by the `make testbed` target as the OpenStack trust anchor for
the lab: the target defaults `OS_CACERT` to `$(CURDIR)/contrib/testbed.pem` (see
`Makefile:224`) and passes it through as the clouds.yaml `cacert`. Northwatch's
OpenStack enrichment then verifies the testbed Keystone/OpenStack API against
this CA (`config.OpenStackCACert` → `internal/enrich/openstack.go`, which builds
an HTTP client trusting exactly this bundle). The target fails fast if the file
is missing.

What it is **not**:

- It is **not a runtime trust anchor of Northwatch itself.** Northwatch does not
  read `contrib/testbed.pem`; only the `make testbed` convenience target points
  `OS_CACERT` at it. A real deployment supplies its own `--os-cacert` /
  `OS_CACERT` (or none) per environment.
- It is **not a production trust anchor.** Do not confuse this lab CA with the
  per-environment CAs your own OpenStack or OVSDB endpoints use. It only trusts
  the OSISM testbed.

For the full `make testbed` surface and its overridable variables, see
[Make targets](https://b42labs.github.io/northwatch/reference/make-targets) and
[Enrich with OpenStack](https://b42labs.github.io/northwatch/how-to/enrich-with-openstack).
