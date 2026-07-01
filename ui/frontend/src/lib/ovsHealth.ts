// Pure helpers for the fleet-wide OVS health overview: the summary tiles and
// the per-chassis problem count that flags a chassis in the picker. Kept free of
// Svelte so the logic is unit-testable in isolation (mirrors lib/ovsDetail.ts,
// lib/status.ts).

import type { Variant } from './status';
import type { OvsChassisHealth, OvsFleetHealth } from './api';

/** chassisProblems is the number of interfaces on a chassis that are down or
 * erroring — the count that flags a connected chassis in the picker beyond its
 * connection dot. */
export function chassisProblems(m: OvsChassisHealth): number {
  return m.down_interfaces + m.error_interfaces;
}

/** HealthTile is one overview summary tile. Its shape matches StatTiles' Tile so
 * a HealthTile[] can be passed straight through. */
export interface HealthTile {
  label: string;
  value: string | number;
  variant?: Variant;
  hint?: string;
}

/** countVariant flags a non-zero count with the given variant, else neutral. */
function countVariant(n: number, variant: Variant): Variant {
  return n > 0 ? variant : 'neutral';
}

/** fleetTiles derives the overview summary tiles from the fleet health: the
 * connection counts, the fleet-wide bridge/port/interface totals, and the down
 * and erroring interface indicators. Unreachable, Down and Errored turn from
 * neutral to a warning/error variant only when their count is non-zero, so a
 * healthy fleet reads as calm and a problem stands out at a glance. */
export function fleetTiles(h: OvsFleetHealth): HealthTile[] {
  return [
    { label: 'Chassis', value: h.chassis },
    { label: 'Connected', value: h.connected, variant: 'success' },
    {
      label: 'Unreachable',
      value: h.unreachable,
      variant: countVariant(h.unreachable, 'warning'),
    },
    { label: 'Bridges', value: h.bridges },
    { label: 'Ports', value: h.ports },
    { label: 'Interfaces', value: h.interfaces },
    {
      label: 'Down',
      value: h.down_interfaces,
      variant: countVariant(h.down_interfaces, 'error'),
      hint: 'interfaces with link_state=down',
    },
    {
      label: 'Errored',
      value: h.error_interfaces,
      variant: countVariant(h.error_interfaces, 'error'),
      hint: 'interfaces with rx/tx errors or drops',
    },
  ];
}
