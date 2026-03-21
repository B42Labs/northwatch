import { describe, it, expect } from 'vitest';
import {
  writableTables,
  isWritableTable,
  findWritableTable,
} from './writableTables';

describe('writableTables', () => {
  it('contains 14 writable tables', () => {
    expect(writableTables).toHaveLength(14);
  });

  it('includes core NB tables', () => {
    const names = writableTables.map((t) => t.ovsdbName);
    expect(names).toContain('Logical_Switch');
    expect(names).toContain('Logical_Switch_Port');
    expect(names).toContain('Logical_Router');
    expect(names).toContain('ACL');
    expect(names).toContain('NAT');
  });
});

describe('isWritableTable', () => {
  it('returns true for writable tables', () => {
    expect(isWritableTable('Logical_Switch')).toBe(true);
    expect(isWritableTable('ACL')).toBe(true);
    expect(isWritableTable('Static_MAC_Binding')).toBe(true);
  });

  it('returns false for non-writable tables', () => {
    expect(isWritableTable('Chassis')).toBe(false);
    expect(isWritableTable('Port_Binding')).toBe(false);
    expect(isWritableTable('Logical_Flow')).toBe(false);
  });
});

describe('findWritableTable', () => {
  it('finds a writable table by OVSDB name', () => {
    const t = findWritableTable('Logical_Switch');
    expect(t).toBeDefined();
    expect(t!.slug).toBe('logical-switches');
    expect(t!.label).toBe('Logical Switches');
  });

  it('returns undefined for non-writable tables', () => {
    expect(findWritableTable('Chassis')).toBeUndefined();
  });
});
