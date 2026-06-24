# Search with Omnisearch

Omnisearch is the primary entry point for debugging. A single query runs across
the Northbound tables, the Southbound tables and any enrichment caches in
parallel, and returns results grouped by entity type.

## Run a search

```bash
curl -s 'http://localhost:8080/api/v1/search?q=10.0.0.42'
```

In the UI this is the search field at the top of every page; the grouped results
are clickable and link straight to the entity's correlated view.

## What the input can be

Omnisearch detects the type of `q` automatically:

| Input | Example |
|---|---|
| IPv4 / IPv6 address | `q=10.0.0.42` |
| MAC address (full or partial) | `q=fa:16:3e:aa:bb` |
| UUID (full or fragment) | `q=a1b2c3d4` |
| Free-text name | `q=web-server-01` |

## What it searches

The engine indexes the highest-value tables on both databases, including:

- **Northbound:** `Logical_Switch`, `Logical_Switch_Port`, `Logical_Router`,
  `Logical_Router_Port`, `ACL`, `NAT`, `Address_Set`, `Port_Group`,
  `Load_Balancer`, `DHCP_Options`, static routes and policies, `DNS`.
- **Southbound:** `Chassis`, `Port_Binding`, `Logical_Flow`, `Datapath_Binding`,
  `Encap`, `MAC_Binding`, `FDB`, `Address_Set`, `DNS`, `Load_Balancer`.

If you skipped a table at startup with `--monitor-skip-tables` (for example
`Logical_Flow`), it is not in the cache and therefore not searchable. See
[Tune the initial load](/how-to/tune-the-initial-load).

## In a multi-cluster setup

Search a specific cluster by prefixing the route:

```bash
curl -s 'http://localhost:8080/api/v1/clusters/staging/search?q=web-server-01'
```

## Next steps

- Follow a result down the correlation chain:
  [Investigate with Omnisearch](/tutorials/investigate-with-omnisearch).
- Understand how the engine and correlation fit together:
  [Correlation & search](/explanation/correlation-and-search).
