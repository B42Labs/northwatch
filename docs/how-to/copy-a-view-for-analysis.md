# Copy a view for analysis

Debugging OVN with an AI assistant usually means pasting state into it. Copying
one field at a time loses what made the field meaningful: which cluster it came
from, whether the data was live, which filters were active. Northwatch copies a
whole view in one click, with that context attached.

## Where the buttons are

Two buttons, `Copy MD` and `Copy JSON`, sit in the header of the pages that show
a single fault:

- the six correlated profiles (logical switch, logical router, chassis, logical
  switch port, logical router port, port binding),
- the raw record view and the OVS row view,
- the seven debug pages (packet trace, flow diff, connectivity, port
  diagnostics, ACL audit, stale entries, next-hop MAC).

Every collapsible `Raw JSON` block also carries its own `Copy` button, which
copies that record alone.

List and dashboard pages have no copy action. They can hold thousands of rows,
which is more than a chat paste is good for; open the entity you care about and
copy from there.

## The two formats

`Copy MD` produces Markdown: a context header followed by the data in a fenced
JSON block. Paste this into a chat.

````markdown
## Northwatch view: Port Diagnostics

- Cluster: prod (Production)
- Mode: live
- Route: /debug/port-diagnostics
- Query: severity=error
- Copied at: 2026-09-22T08:30:00.000Z
- Masked: no

```json
{
  "filters": { "search": "", "severity": "error" },
  "result": { "total": 42, "healthy": 40, "warning": 0, "error": 2, "ports": [] }
}
```
````

In snapshot mode the `Mode` line names the snapshot's creation time, so a paste
from an offline capture is never mistaken for live state.

`Copy JSON` produces the data alone, with no header, for a script or a tool that
expects machine-readable input.

A diagnostics page copies its inputs and active filters next to the result. The
connectivity checker includes the two port UUIDs it was given, the packet trace
its source port and destination IP. Whoever reads the paste can reproduce it.

## Masking addresses

The `Mask` checkbox next to the buttons replaces addresses with stable
placeholders before copying. It is off by default, and the choice is remembered
in the browser.

With masking on:

- MAC addresses become `mac-1`, `mac-2`, and so on.
- IPv6 addresses become `ip6-1`, keeping any prefix length: `fd00::1/64` becomes
  `ip6-1/64`.
- IPv4 addresses become `ip-1`, keeping any prefix length or port:
  `10.0.0.5/24` becomes `ip-1/24`, and `192.168.1.1:6642` becomes `ip-1:6642`.
- Values stored under a `hostname` key become `host-1`.

The same address always maps to the same placeholder within one copy, so an
assistant can still follow which port talks to which.

Masking is a convenience, not a guarantee. It does not touch UUIDs, tunnel keys,
external IDs or logical port names, because those are the identifiers that make
a view analysable at all. Hostnames are replaced only under a `hostname` key: an
OVN entity name such as `sw0-port1` cannot be told apart from a hostname by
pattern, so nothing else is guessed at.

Leave masking off for ordinary debugging. Addresses are usually the thing being
analysed.

## Copying over plain HTTP

The browser's clipboard API needs a secure context, which a lab host serving
Northwatch over plain HTTP does not provide. The buttons fall back to an older
copy mechanism there, so they work either way. A button that says `Failed`
instead of `Copied` means the browser refused both paths; copy from the raw JSON
block by hand in that case.
