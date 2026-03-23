import { describe, it, expect } from 'vitest';
import {
  writableTables,
  isWritableTable,
  findWritableTable,
} from './writableTables';

describe('writableTables', () => {
  it('contains 19 writable tables (16 NB + 3 SB)', () => {
    expect(writableTables).toHaveLength(19);
  });

  it('includes core NB tables', () => {
    const names = writableTables.map((t) => t.ovsdbName);
    expect(names).toContain('Logical_Switch');
    expect(names).toContain('Logical_Switch_Port');
    expect(names).toContain('Logical_Router');
    expect(names).toContain('ACL');
    expect(names).toContain('NAT');
  });

  it('marks SB tables as deleteOnly', () => {
    const sbTables = writableTables.filter((t) => t.deleteOnly);
    expect(sbTables).toHaveLength(3);
    const names = sbTables.map((t) => t.ovsdbName);
    expect(names).toContain('MAC_Binding');
    expect(names).toContain('FDB');
    expect(names).toContain('Port_Binding');
  });

  it('does not mark NB tables as deleteOnly', () => {
    const nbTables = writableTables.filter((t) => !t.deleteOnly);
    expect(nbTables).toHaveLength(16);
  });
});

describe('isWritableTable', () => {
  it('returns true for writable NB tables', () => {
    expect(isWritableTable('Logical_Switch')).toBe(true);
    expect(isWritableTable('ACL')).toBe(true);
    expect(isWritableTable('Static_MAC_Binding')).toBe(true);
  });

  it('returns true for writable SB tables', () => {
    expect(isWritableTable('MAC_Binding')).toBe(true);
    expect(isWritableTable('FDB')).toBe(true);
    expect(isWritableTable('Port_Binding')).toBe(true);
  });

  it('returns false for non-writable tables', () => {
    expect(isWritableTable('Chassis')).toBe(false);
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
