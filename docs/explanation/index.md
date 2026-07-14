# Explanation

Understanding-oriented background. These pages explain how Northwatch works and
why it is built the way it is — read them to build a mental model, not to perform
a task.

- [Architecture](/explanation/architecture) — the single binary, the libovsdb
  in-memory cache, and the path from OVSDB monitor to UI.
- [Correlation & search](/explanation/correlation-and-search) — how Northwatch
  links Northbound intent to Southbound realization, and how Omnisearch uses
  those links.
- [The capability model](/explanation/capability-model) — why access is modelled
  as additive capabilities rather than modes, and what the authentication
  boundary covers.
- [Enrichment](/explanation/enrichment) — pluggable providers, name resolution,
  and the caching strategy.
- [History & snapshots](/explanation/history-and-snapshots) — the SQLite event
  log, periodic snapshots, and the offline-replay design.
- [Write safety](/explanation/write-safety) — the plan/preview/apply workflow,
  the audit log, and the safeguards around mutation.
- [Large deployments](/explanation/large-deployments) — why the initial monitor
  is the heaviest moment and how the tuning knobs bound it.
- [Roadmap](/explanation/roadmap) — what exists today and what is planned.

For step-by-step instructions, start with the [Tutorials](/tutorials/) or the
[How-to guides](/how-to/).
