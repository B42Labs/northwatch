// Pure helpers for the per-row OVS detail view. They classify a row's
// map-typed fields into readable key/value sections and derive the headline
// interface signals a debugging engineer scans first. Kept free of Svelte so
// the logic is unit-testable in isolation (mirrors lib/status.ts, lib/nav.ts).

import type { Variant } from './status';

/** isMapValue reports whether v is a non-null, non-array object — i.e. a
 * map-typed OVSDB field that serializes to a JSON object. */
export function isMapValue(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** OvsMapSection is one map-typed field rendered as a key/value section. */
export interface OvsMapSection {
  key: string;
  data: Record<string, string>;
}

/** mapSections returns every non-empty map-typed field of a row as a key/value
 * section, sorted by field name with each section's keys sorted and values
 * stringified — so numeric maps (e.g. Interface.statistics) render as text.
 * Empty maps and null fields are skipped so a section is shown only when it
 * carries data. */
export function mapSections(row: Record<string, unknown>): OvsMapSection[] {
  const sections: OvsMapSection[] = [];
  for (const [key, value] of Object.entries(row)) {
    if (!isMapValue(value)) continue;
    const keys = Object.keys(value).sort();
    if (keys.length === 0) continue;
    const data: Record<string, string> = {};
    for (const k of keys) {
      data[k] = String(value[k]);
    }
    sections.push({ key, data });
  }
  return sections.sort((a, b) => a.key.localeCompare(b.key));
}

/** Signal is one headline interface metric. Its shape matches StatTiles' Tile
 * so a Signal[] can be passed straight through. */
export interface Signal {
  label: string;
  value: string | number;
  variant?: Variant;
}

/** linkStateVariant maps an interface link_state to a status variant. */
export function linkStateVariant(s?: string): Variant {
  if (s === 'up') return 'success';
  if (s === 'down') return 'error';
  return 'neutral';
}

/** counterVariant flags a non-zero error/drop counter as an error. */
export function counterVariant(n: number): Variant {
  return n > 0 ? 'error' : 'neutral';
}

/** formatLinkSpeed renders a link speed (bits/s) as a human string such as
 * "10 Gbps" or "1 Mbps", or "-" when the speed is absent or zero. */
export function formatLinkSpeed(n?: number | null): string {
  if (!n || n <= 0) return '-';
  const units: [number, string][] = [
    [1e9, 'Gbps'],
    [1e6, 'Mbps'],
    [1e3, 'Kbps'],
  ];
  for (const [factor, unit] of units) {
    if (n >= factor) {
      const val = n / factor;
      const str = Number.isInteger(val) ? String(val) : val.toFixed(1);
      return `${str} ${unit}`;
    }
  }
  return `${n} bps`;
}

// Standard OVS interface statistics keys surfaced as headline counters; a
// driver that omits a key falls back to 0 so the tile is still shown.
const COUNTER_KEYS = ['rx_errors', 'tx_errors', 'rx_dropped', 'tx_dropped'];

function numberOrDash(v: unknown): string | number {
  return typeof v === 'number' ? v : '-';
}

function stringOrDash(v: unknown): string {
  return typeof v === 'string' && v !== '' ? v : '-';
}

/** interfaceSignals derives the headline interface signals a debugging
 * engineer scans first: link state, speed, mtu, ofport, mac_in_use, error, and
 * the rx/tx error and drop counters read from the statistics map. */
export function interfaceSignals(row: Record<string, unknown>): Signal[] {
  const signals: Signal[] = [];

  const linkState =
    typeof row.link_state === 'string' ? row.link_state : undefined;
  signals.push({
    label: 'link_state',
    value: linkState ?? '-',
    variant: linkStateVariant(linkState),
  });

  const linkSpeed = typeof row.link_speed === 'number' ? row.link_speed : null;
  signals.push({ label: 'speed', value: formatLinkSpeed(linkSpeed) });

  signals.push({ label: 'mtu', value: numberOrDash(row.mtu) });
  signals.push({ label: 'ofport', value: numberOrDash(row.ofport) });
  signals.push({ label: 'mac_in_use', value: stringOrDash(row.mac_in_use) });

  const error = typeof row.error === 'string' ? row.error : '';
  signals.push({
    label: 'error',
    value: error || 'none',
    variant: error ? 'error' : 'success',
  });

  const stats = isMapValue(row.statistics) ? row.statistics : {};
  for (const key of COUNTER_KEYS) {
    const n = typeof stats[key] === 'number' ? (stats[key] as number) : 0;
    signals.push({ label: key, value: n, variant: counterVariant(n) });
  }

  return signals;
}
