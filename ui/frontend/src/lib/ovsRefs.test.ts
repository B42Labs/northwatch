import { describe, it, expect } from 'vitest';
import {
  buildLabelIndex,
  entityLabel,
  ovsRefHref,
  ovsRefLabel,
  referenceTargets,
} from './ovsRefs';

describe('entityLabel', () => {
  it('prefers a non-empty name', () => {
    expect(entityLabel({ name: 'br-int', target: 'tcp:1.2.3.4:6653' })).toBe(
      'br-int',
    );
  });

  it('falls back to target when name is absent or empty', () => {
    // Controller/Manager rows carry a `target`, not a `name`.
    expect(entityLabel({ target: 'tcp:1.2.3.4:6653' })).toBe(
      'tcp:1.2.3.4:6653',
    );
    expect(entityLabel({ name: '', target: 'ptcp:6640' })).toBe('ptcp:6640');
  });

  it('returns undefined when neither name nor target is a usable string', () => {
    expect(entityLabel({})).toBeUndefined();
    expect(entityLabel({ name: '', target: '' })).toBeUndefined();
    expect(entityLabel({ name: 42, target: null })).toBeUndefined();
  });
});

describe('referenceTargets', () => {
  it('returns the labelled fetch set per source table', () => {
    // Nameless targets (netflow, sflow, ipfix, autoattach, ssl, qos) are
    // dropped — fetching them would add a request without yielding a label.
    expect(referenceTargets('bridge').sort()).toEqual([
      'controller',
      'mirror',
      'port',
    ]);
    expect(referenceTargets('port')).toEqual(['interface']);
    expect(referenceTargets('open-vswitch').sort()).toEqual([
      'bridge',
      'manager',
    ]);
    expect(referenceTargets('mirror')).toEqual(['port']);
  });

  it('returns an empty list for tables with no labelled references', () => {
    // Interface is a leaf in the Bridge → Port → Interface walk, and unknown
    // tables have no reference columns at all.
    expect(referenceTargets('interface')).toEqual([]);
    expect(referenceTargets('nonexistent')).toEqual([]);
  });

  it('does not resolve inherited Object.prototype members as tables', () => {
    // A caller-supplied slug that names a prototype member must not walk the
    // chain onto Object.prototype.
    expect(referenceTargets('constructor')).toEqual([]);
    expect(referenceTargets('toString')).toEqual([]);
    expect(referenceTargets('__proto__')).toEqual([]);
  });
});

describe('buildLabelIndex', () => {
  it('maps _uuid to label, skipping rows without a usable _uuid', () => {
    const index = buildLabelIndex([
      {
        slug: 'port',
        rows: [
          { _uuid: 'u1', name: 'eth0' },
          { _uuid: '', name: 'dropped' },
          { name: 'no-uuid' },
        ],
      },
      { slug: 'controller', rows: [{ _uuid: 'u2', target: 'tcp:x:6653' }] },
    ]);

    expect(index.get('u1')).toBe('eth0');
    expect(index.get('u2')).toBe('tcp:x:6653');
    // Rows with an empty or missing _uuid never enter the index.
    expect(index.size).toBe(2);
  });

  it('skips a nameless row', () => {
    const index = buildLabelIndex([{ slug: 'ssl', rows: [{ _uuid: 'u3' }] }]);
    expect(index.get('u3')).toBeUndefined();
    expect(index.size).toBe(0);
  });
});

describe('ovsRefHref', () => {
  it('returns undefined for a non-reference column', () => {
    expect(ovsRefHref('hv1', 'bridge')('name')).toBeUndefined();
  });

  it('builds the detail href for a reference column, encoding chassis and uuid', () => {
    const href = ovsRefHref('hv 1', 'bridge')('ports');
    expect(href).toBeDefined();
    expect(href!('uu/id')).toBe('/ovs/hv%201/port/uu%2Fid');
  });

  it('treats inherited Object.prototype members as non-reference columns', () => {
    // Columns derived from Object.keys(row) must never resolve to a prototype
    // member (constructor, toString), which would build a malformed href.
    const factory = ovsRefHref('hv1', 'bridge');
    expect(factory('constructor')).toBeUndefined();
    expect(factory('toString')).toBeUndefined();
    expect(factory('hasOwnProperty')).toBeUndefined();
  });
});

describe('ovsRefLabel', () => {
  const index = buildLabelIndex([
    { slug: 'port', rows: [{ _uuid: 'p1', name: 'eth0' }] },
  ]);

  it('returns undefined for a non-reference column', () => {
    expect(ovsRefLabel(index, 'bridge')('name')).toBeUndefined();
  });

  it('resolves a reference UUID to its indexed label', () => {
    const resolve = ovsRefLabel(index, 'bridge')('ports');
    expect(resolve).toBeDefined();
    expect(resolve!('p1')).toBe('eth0');
  });

  it('returns null for a UUID missing from the index so the renderer falls back to the short UUID', () => {
    const resolve = ovsRefLabel(index, 'bridge')('ports');
    expect(resolve!('unknown')).toBeNull();
  });

  it('treats inherited Object.prototype members as non-reference columns', () => {
    // A prototype member must yield undefined, not a live resolver.
    const factory = ovsRefLabel(index, 'bridge');
    expect(factory('constructor')).toBeUndefined();
    expect(factory('toString')).toBeUndefined();
  });
});
