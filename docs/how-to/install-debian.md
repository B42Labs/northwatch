# Install on Debian/Ubuntu

Install Northwatch from the release `.deb` so it runs as a managed systemd
service — started on boot, restarted on failure, logging to journald, and
running as a dedicated unprivileged `northwatch` user. The binary is fully
static with the UI embedded, so the package depends on nothing but `adduser`
(which it needs to create that user).

Every tagged release attaches `northwatch_<version>_amd64.deb` and
`northwatch_<version>_arm64.deb`, each with a cosign `.sig` and `.pem`, plus a
shared `checksums.txt`.

## Download and verify

Download the package for your architecture and its signature material from the
[releases page](https://github.com/B42Labs/northwatch/releases). Verify the
checksum:

```bash
sha256sum -c checksums.txt --ignore-missing
```

Then verify the cosign signature (the release is keyless-signed by the GitHub
Actions workflow):

```bash
cosign verify-blob \
  --certificate northwatch_<version>_amd64.deb.pem \
  --signature   northwatch_<version>_amd64.deb.sig \
  --certificate-identity-regexp '^https://github.com/B42Labs/northwatch/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  northwatch_<version>_amd64.deb
```

## Install

```bash
sudo apt install ./northwatch_<version>_amd64.deb
```

Installing the package lands the files listed below and creates the `northwatch`
system user and group. The service is **not** started automatically — configure
it first.

| Path | Content |
|---|---|
| `/usr/bin/northwatch` | static binary (embedded UI) |
| `/lib/systemd/system/northwatch.service` | hardened unit, runs as `northwatch` |
| `/etc/default/northwatch` | env config (preserved on upgrade) |
| `/var/lib/northwatch/` | state directory, `0750 northwatch:northwatch` — SQLite history DB |

## Configure

Edit `/etc/default/northwatch` and set the OVN Northbound and Southbound
addresses (comma-separated for Raft failover):

```sh
NORTHWATCH_OVN_NB_ADDR=tcp:10.0.0.1:6641,tcp:10.0.0.2:6641,tcp:10.0.0.3:6641
NORTHWATCH_OVN_SB_ADDR=tcp:10.0.0.1:6642,tcp:10.0.0.2:6642,tcp:10.0.0.3:6642
```

Every other knob is listed there, commented out with its default. The history
DB is preset to `/var/lib/northwatch/history.db` so it lands in the state
directory. For the full surface of every variable, see
[CLI flags & env vars](/reference/cli).

::: warning No built-in authentication
Northwatch has no user management. Anyone who can reach the API can read your
OVN deployment (and mutate it if you set `NORTHWATCH_WRITE_ENABLED=true`). Bind
it to localhost (`NORTHWATCH_LISTEN=127.0.0.1:8080`) behind a reverse proxy that
handles authentication, and restrict network access. See
[The capability model](/explanation/capability-model).
:::

## Start the service

```bash
sudo systemctl enable --now northwatch
```

Check that it came up and is serving the API/UI:

```bash
systemctl status northwatch
journalctl -u northwatch -f
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/api/v1/capabilities
```

## Upgrade, remove and purge

Installing a newer `.deb` over an existing install **preserves your
`/etc/default/northwatch` edits** — it is a conffile, so dpkg never clobbers it
(a new default ships as `northwatch.default.dpkg-dist` for you to diff).

```bash
sudo apt install ./northwatch_<newer-version>_amd64.deb   # upgrade
sudo apt remove northwatch                                # remove, keeps state
sudo apt purge northwatch                                 # purge, drops state
```

`remove` stops and disables the service but keeps `/var/lib/northwatch` (your
history DB). `purge` additionally removes `/var/lib/northwatch`. The
`northwatch` system user and group are intentionally retained on purge — an
orphaned system account is harmless and avoids touching files it may own
elsewhere.
