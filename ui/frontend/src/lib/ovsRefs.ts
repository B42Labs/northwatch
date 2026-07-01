// Pure helpers that resolve OVS UUID reference columns to human labels and
// detail-view hrefs, so a per-chassis Bridge → Port → Interface can be walked
// by name instead of by matching raw UUIDs by hand. Kept free of Svelte so the
// logic is unit-testable in isolation (mirrors lib/ovsDetail.ts, lib/tables.ts).

// OVS_REFERENCES maps an OVS table slug to its reference columns (column name →
// target table slug), for the single-UUID and set-typed references drawn from
// the Open_vSwitch schema. Map-typed references (Bridge.flow_tables,
// Open_vSwitch.datapaths, QoS.queues) serialize to JSON objects rather than
// UUID scalars/arrays, so they are deliberately excluded. Every target slug
// matches an OVS_TABLES entry, so an href can be built even when the target row
// was not fetched for the current chassis.
export const OVS_REFERENCES: Record<string, Record<string, string>> = {
  'open-vswitch': {
    bridges: 'bridge',
    manager_options: 'manager',
    ssl: 'ssl',
  },
  bridge: {
    ports: 'port',
    mirrors: 'mirror',
    controller: 'controller',
    netflow: 'netflow',
    sflow: 'sflow',
    ipfix: 'ipfix',
    auto_attach: 'autoattach',
  },
  port: {
    interfaces: 'interface',
    qos: 'qos',
  },
  mirror: {
    select_src_port: 'port',
    select_dst_port: 'port',
    output_port: 'port',
  },
};

// LABELLED_TABLES are the target tables that carry a human label (a `name`, or
// a `target` for controller/manager). referenceTargets restricts its fetches to
// these: fetching a nameless target (ssl, netflow, …) would add a request
// without yielding a label, so those references still link but render the short
// UUID.
const LABELLED_TABLES = new Set([
  'interface',
  'port',
  'bridge',
  'mirror',
  'controller',
  'manager',
]);

/** entityLabel returns the human label for an OVS row: its non-empty `name`,
 * else its non-empty `target` (controller/manager have no name), else
 * undefined. */
export function entityLabel(row: Record<string, unknown>): string | undefined {
  const name = row.name;
  if (typeof name === 'string' && name !== '') return name;
  const target = row.target;
  if (typeof target === 'string' && target !== '') return target;
  return undefined;
}

/** referenceTargets returns the distinct target table slugs worth fetching to
 * label the given table's reference columns — restricted to the tables that
 * expose a name/target. Returns [] for tables with no labelled references. */
export function referenceTargets(tableSlug: string): string[] {
  // Object.hasOwn guards against tableSlug hitting an inherited Object.prototype
  // member (constructor, toString, __proto__) since the slug is caller-supplied.
  const refs = Object.hasOwn(OVS_REFERENCES, tableSlug)
    ? OVS_REFERENCES[tableSlug]
    : undefined;
  if (!refs) return [];
  const targets = new Set<string>();
  for (const target of Object.values(refs)) {
    if (LABELLED_TABLES.has(target)) targets.add(target);
  }
  return [...targets];
}

/** buildLabelIndex indexes fetched rows by `_uuid` → label so a reference UUID
 * can be resolved to its target's label. Rows lacking a non-empty string
 * `_uuid`, or lacking a label, are skipped. */
export function buildLabelIndex(
  tables: { slug: string; rows: Record<string, unknown>[] }[],
): Map<string, string> {
  const index = new Map<string, string>();
  for (const { rows } of tables) {
    for (const row of rows) {
      const uuid = row._uuid;
      if (typeof uuid !== 'string' || uuid === '') continue;
      const label = entityLabel(row);
      if (label === undefined) continue;
      index.set(uuid, label);
    }
  }
  return index;
}

/** ovsRefHref returns a per-column href factory for the given table, building
 * `/ovs/{chassis}/{target}/{uuid}` for each reference column — so a reference
 * links even when its target row was not fetched. Returns undefined for a
 * non-reference column, matching the DataTable/PropertyCard refHref contract. */
export function ovsRefHref(
  chassis: string,
  tableSlug: string,
): (column: string) => ((uuid: string) => string | null) | undefined {
  const refs = Object.hasOwn(OVS_REFERENCES, tableSlug)
    ? OVS_REFERENCES[tableSlug]
    : {};
  return (column: string) => {
    // Object.hasOwn keeps inherited members (constructor, toString) from
    // resolving to a live prototype value and building a malformed href.
    const target = Object.hasOwn(refs, column) ? refs[column] : undefined;
    if (!target) return undefined;
    return (uuid: string) =>
      `/ovs/${encodeURIComponent(chassis)}/${target}/${encodeURIComponent(uuid)}`;
  };
}

/** ovsRefLabel returns a per-column label factory for the given table,
 * resolving each reference UUID to its indexed label (null when absent, so the
 * renderer falls back to the short UUID). Returns undefined for a non-reference
 * column, matching the DataTable/PropertyCard refLabel contract. */
export function ovsRefLabel(
  index: Map<string, string>,
  tableSlug: string,
): (column: string) => ((uuid: string) => string | null) | undefined {
  const refs = Object.hasOwn(OVS_REFERENCES, tableSlug)
    ? OVS_REFERENCES[tableSlug]
    : {};
  return (column: string) => {
    // Object.hasOwn keeps inherited members (constructor, toString) from
    // resolving to a live prototype value and returning a bogus resolver.
    if (!Object.hasOwn(refs, column)) return undefined;
    if (!refs[column]) return undefined;
    return (uuid: string) => index.get(uuid) ?? null;
  };
}
