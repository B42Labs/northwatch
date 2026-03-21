import { describe, it, expect } from 'vitest';
import {
  databases,
  findTable,
  tableSlugFromOvsdbName,
  getCorrelatedRoute,
  getCorrelatedListRoute,
  correlatedViews,
} from './tables';

describe('databases', () => {
  it('has nb and sb databases', () => {
    const keys = databases.map((d) => d.key);
    expect(keys).toContain('nb');
    expect(keys).toContain('sb');
  });

  it('every table has a slug and at least one primary column', () => {
    for (const db of databases) {
      for (const table of db.tables) {
        expect(table.slug).toBeTruthy();
        expect(table.primaryColumns.length).toBeGreaterThan(0);
        expect(table.primaryColumns).toContain('_uuid');
      }
    }
  });

  it('has no duplicate slugs within a database', () => {
    for (const db of databases) {
      const slugs = db.tables.map((t) => t.slug);
      expect(new Set(slugs).size).toBe(slugs.length);
    }
  });
});

describe('findTable', () => {
  it('finds a known NB table', () => {
    const table = findTable('nb', 'logical-switches');
    expect(table).toBeDefined();
    expect(table!.label).toBe('Logical Switches');
  });

  it('finds a known SB table', () => {
    const table = findTable('sb', 'chassis');
    expect(table).toBeDefined();
    expect(table!.label).toBe('Chassis');
  });

  it('returns undefined for unknown table', () => {
    expect(findTable('nb', 'nonexistent')).toBeUndefined();
  });

  it('returns undefined for unknown database', () => {
    expect(findTable('xx', 'logical-switches')).toBeUndefined();
  });
});

describe('tableSlugFromOvsdbName', () => {
  it('maps standard OVSDB names', () => {
    expect(tableSlugFromOvsdbName('Logical_Switch')).toBe('logical-switches');
    expect(tableSlugFromOvsdbName('Logical_Switch_Port')).toBe(
      'logical-switch-ports',
    );
    expect(tableSlugFromOvsdbName('Logical_Router')).toBe('logical-routers');
    expect(tableSlugFromOvsdbName('Port_Binding')).toBe('port-bindings');
    expect(tableSlugFromOvsdbName('Datapath_Binding')).toBe(
      'datapath-bindings',
    );
    expect(tableSlugFromOvsdbName('Chassis_Private')).toBe('chassis-private');
  });

  it('maps acronym OVSDB names', () => {
    expect(tableSlugFromOvsdbName('ACL')).toBe('acls');
    expect(tableSlugFromOvsdbName('NAT')).toBe('nats');
    expect(tableSlugFromOvsdbName('DNS')).toBe('dns');
    expect(tableSlugFromOvsdbName('FDB')).toBe('fdb');
    expect(tableSlugFromOvsdbName('BFD')).toBe('bfd');
    expect(tableSlugFromOvsdbName('QoS')).toBe('qos');
  });

  it('maps singular OVSDB names to plural slugs', () => {
    expect(tableSlugFromOvsdbName('Encap')).toBe('encaps');
    expect(tableSlugFromOvsdbName('Meter')).toBe('meters');
    expect(tableSlugFromOvsdbName('Mirror')).toBe('mirrors');
  });

  it('maps compound names', () => {
    expect(tableSlugFromOvsdbName('MAC_Binding')).toBe('mac-bindings');
    expect(tableSlugFromOvsdbName('DHCP_Options')).toBe('dhcp-options');
    expect(tableSlugFromOvsdbName('NB_Global')).toBe('nb-global');
    expect(tableSlugFromOvsdbName('SB_Global')).toBe('sb-global');
    expect(tableSlugFromOvsdbName('HA_Chassis_Group')).toBe(
      'ha-chassis-groups',
    );
  });

  it('falls back for unknown names', () => {
    expect(tableSlugFromOvsdbName('Some_New_Table')).toBe('some-new-table');
  });
});

describe('getCorrelatedRoute', () => {
  it('returns route for correlated tables', () => {
    expect(getCorrelatedRoute('nb', 'logical-switches')).toBe(
      '/correlated/logical-switches',
    );
    expect(getCorrelatedRoute('nb', 'logical-routers')).toBe(
      '/correlated/logical-routers',
    );
    expect(getCorrelatedRoute('sb', 'chassis')).toBe('/correlated/chassis');
    expect(getCorrelatedRoute('sb', 'port-bindings')).toBe(
      '/correlated/port-bindings',
    );
  });

  it('returns null for non-correlated tables', () => {
    expect(getCorrelatedRoute('nb', 'acls')).toBeNull();
    expect(getCorrelatedRoute('sb', 'logical-flows')).toBeNull();
  });
});

describe('getCorrelatedListRoute', () => {
  it('returns route only for tables with list views', () => {
    expect(getCorrelatedListRoute('nb', 'logical-switches')).toBe(
      '/correlated/logical-switches',
    );
    expect(getCorrelatedListRoute('nb', 'logical-routers')).toBe(
      '/correlated/logical-routers',
    );
    expect(getCorrelatedListRoute('sb', 'chassis')).toBe('/correlated/chassis');
  });

  it('returns null for detail-only correlated tables', () => {
    expect(getCorrelatedListRoute('nb', 'logical-switch-ports')).toBeNull();
    expect(getCorrelatedListRoute('nb', 'logical-router-ports')).toBeNull();
    expect(getCorrelatedListRoute('sb', 'port-bindings')).toBeNull();
  });
});

describe('correlatedViews', () => {
  it('contains the expected views', () => {
    const slugs = correlatedViews.map((v) => v.slug);
    expect(slugs).toContain('logical-switches');
    expect(slugs).toContain('logical-routers');
    expect(slugs).toContain('chassis');
  });
});
