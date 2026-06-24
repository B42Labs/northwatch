---
layout: home

hero:
  name: Northwatch
  text: See your OVN deployment whole
  tagline: >-
    A single service that connects to the OVN Northbound and Southbound OVSDB
    databases and gives you a live, searchable, correlated view — browse,
    visualize, debug, trace, and monitor from one web UI and REST API.
  actions:
    - theme: brand
      text: Get started
      link: /tutorials/getting-started
    - theme: alt
      text: CLI reference
      link: /reference/cli
    - theme: alt
      text: View on GitHub
      link: https://github.com/B42Labs/northwatch

features:
  - title: Tutorials
    details: >-
      Learning-oriented, guided paths. Bring up a throwaway OVN lab, point
      Northwatch at it, and walk through your first investigation end to end.
    link: /tutorials/
  - title: How-to guides
    details: >-
      Task-oriented recipes. Connect to a Raft cluster, enrich with OpenStack,
      trace a packet, diagnose a port binding, capture a snapshot, enable
      writes, and more.
    link: /how-to/
  - title: Reference
    details: >-
      Information-oriented lookup. Every CLI flag and environment variable, the
      config-file schema, the HTTP API surface, capabilities, and the
      Prometheus metrics.
    link: /reference/
  - title: Explanation
    details: >-
      Understanding-oriented background. The architecture, the NB↔SB
      correlation model, the capability model, enrichment, snapshots, and the
      reasoning behind initial-load tuning.
    link: /explanation/
---

This documentation follows the [Diátaxis](https://diataxis.fr/) framework,
which separates documentation into four modes by reader need: **Tutorials**
(learning), **How-to guides** (tasks), **Reference** (lookup), and
**Explanation** (understanding). Use the cards above to jump to the mode that
matches what you need right now.
