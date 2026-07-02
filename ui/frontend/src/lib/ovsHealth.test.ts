import { describe, it, expect } from 'vitest';
import { chassisProblems, fleetTiles } from './ovsHealth';
import type { OvsChassisHealth, OvsFleetHealth } from './api';

function chassis(overrides: Partial<OvsChassisHealth> = {}): OvsChassisHealth {
  return {
    system_id: 'node-0',
    connected: true,
    bridges: 0,
    ports: 0,
    interfaces: 0,
    down_interfaces: 0,
    error_interfaces: 0,
    drop_interfaces: 0,
    ...overrides,
  };
}

function fleet(overrides: Partial<OvsFleetHealth> = {}): OvsFleetHealth {
  return {
    chassis: 0,
    connected: 0,
    unreachable: 0,
    bridges: 0,
    ports: 0,
    interfaces: 0,
    down_interfaces: 0,
    error_interfaces: 0,
    drop_interfaces: 0,
    members: [],
    ...overrides,
  };
}

describe('chassisProblems', () => {
  it('sums down, erroring and dropping interfaces', () => {
    expect(
      chassisProblems(
        chassis({
          down_interfaces: 2,
          error_interfaces: 3,
          drop_interfaces: 1,
        }),
      ),
    ).toBe(6);
  });

  it('is zero when the chassis is healthy', () => {
    expect(chassisProblems(chassis())).toBe(0);
  });
});

describe('fleetTiles', () => {
  it('lays out the nine tiles in order with the fleet values', () => {
    const tiles = fleetTiles(
      fleet({
        chassis: 3,
        connected: 2,
        unreachable: 1,
        bridges: 4,
        ports: 6,
        interfaces: 8,
        down_interfaces: 1,
        error_interfaces: 2,
        drop_interfaces: 5,
      }),
    );

    expect(tiles.map((t) => t.label)).toEqual([
      'Chassis',
      'Connected',
      'Unreachable',
      'Bridges',
      'Ports',
      'Interfaces',
      'Down',
      'Errored',
      'Dropping',
    ]);
    expect(tiles.map((t) => t.value)).toEqual([3, 2, 1, 4, 6, 8, 1, 2, 5]);
  });

  it('escalates the unreachable, down, errored and dropping variants when non-zero', () => {
    const tiles = fleetTiles(
      fleet({
        unreachable: 1,
        down_interfaces: 1,
        error_interfaces: 1,
        drop_interfaces: 1,
      }),
    );
    const byLabel = Object.fromEntries(tiles.map((t) => [t.label, t]));

    expect(byLabel.Connected.variant).toBe('success');
    expect(byLabel.Unreachable.variant).toBe('warning');
    expect(byLabel.Down.variant).toBe('error');
    expect(byLabel.Errored.variant).toBe('error');
    expect(byLabel.Dropping.variant).toBe('warning');
  });

  it('keeps the problem tiles neutral when the fleet is healthy', () => {
    const tiles = fleetTiles(fleet({ connected: 2 }));
    const byLabel = Object.fromEntries(tiles.map((t) => [t.label, t]));

    expect(byLabel.Unreachable.variant).toBe('neutral');
    expect(byLabel.Down.variant).toBe('neutral');
    expect(byLabel.Errored.variant).toBe('neutral');
    expect(byLabel.Dropping.variant).toBe('neutral');
  });
});
