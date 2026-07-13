import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ImpactWarning from './ImpactWarning.svelte';
import type { ImpactEntry, ImpactNode } from '../../lib/writeApi';

function entry(o: {
  index?: number;
  total?: number;
  byTable?: Record<string, number>;
  truncated?: boolean;
  root?: ImpactNode;
}): ImpactEntry {
  return {
    operation_index: o.index ?? 0,
    result: {
      root: o.root ?? {
        database: 'nb',
        table: 'Logical_Switch',
        uuid: 'root-uuid-0000',
        ref_type: 'root',
      },
      summary: {
        total_affected: o.total ?? 0,
        by_table: o.byTable ?? {},
        by_ref_type: {},
        max_depth: 1,
        truncated: o.truncated ?? false,
      },
    },
  };
}

describe('ImpactWarning', () => {
  it('renders the blast radius and per-table breakdown for a single entry', () => {
    render(ImpactWarning, {
      entries: [
        entry({ total: 3, byTable: { Logical_Switch_Port: 2, ACL: 1 } }),
      ],
    });

    expect(screen.getByText(/Delete impacts 3 dependent objects/)).toBeTruthy();
    expect(screen.getByText('2 Logical_Switch_Port')).toBeTruthy();
    expect(screen.getByText('1 ACL')).toBeTruthy();
  });

  it('merges the per-table counts across entries, summed and sorted desc', () => {
    render(ImpactWarning, {
      entries: [
        entry({ index: 0, total: 2, byTable: { ACL: 2 } }),
        entry({ index: 1, total: 3, byTable: { ACL: 2, DHCP_Options: 3 } }),
      ],
    });

    // total_affected sums to 5; ACL merges to 4 (2 + 2), DHCP_Options is 3.
    expect(screen.getByText(/Delete impacts 5 dependent objects/)).toBeTruthy();
    const acl = screen.getByText('4 ACL');
    const dhcp = screen.getByText('3 DHCP_Options');
    expect(acl).toBeTruthy();
    expect(dhcp).toBeTruthy();
    // Higher count first.
    expect(
      acl.compareDocumentPosition(dhcp) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('uses the singular noun for exactly one affected object', () => {
    render(ImpactWarning, {
      entries: [entry({ total: 1, byTable: { ACL: 1 } })],
    });

    expect(
      screen.getByText(/Delete impacts 1 dependent object\b/),
    ).toBeTruthy();
  });

  it('flags truncation and toggles the dependency tree', async () => {
    render(ImpactWarning, {
      entries: [
        entry({
          total: 2,
          byTable: { ACL: 2 },
          truncated: true,
          root: {
            database: 'nb',
            table: 'Logical_Switch',
            uuid: 'abcdef123456',
            name: 'ls0',
            ref_type: 'root',
            children: [
              {
                database: 'nb',
                table: 'ACL',
                uuid: 'child-uuid-9999',
                ref_type: 'strong',
              },
            ],
          },
        }),
      ],
    });

    expect(screen.getByText('truncated')).toBeTruthy();
    // Tree is collapsed initially.
    expect(screen.queryByText('ls0')).toBeNull();

    const toggle = screen.getByRole('button', {
      name: /Show dependency tree/i,
    });
    await fireEvent.click(toggle);

    // Tree now rendered: root name and its cascade child are visible.
    expect(screen.getByText('ls0')).toBeTruthy();
    expect(screen.getByText('cascade')).toBeTruthy();
    expect(
      screen.getByRole('button', { name: /Hide dependency tree/i }),
    ).toBeTruthy();
  });
});
