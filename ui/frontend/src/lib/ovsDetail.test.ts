import { describe, it, expect } from 'vitest';
import {
  counterVariant,
  formatLinkSpeed,
  interfaceSignals,
  isMapValue,
  linkStateVariant,
  mapSections,
} from './ovsDetail';

describe('isMapValue', () => {
  it('recognizes plain objects, including the empty object', () => {
    expect(isMapValue({})).toBe(true);
    expect(isMapValue({ a: 'b' })).toBe(true);
  });

  it('rejects null, arrays, and primitives', () => {
    expect(isMapValue(null)).toBe(false);
    expect(isMapValue(undefined)).toBe(false);
    expect(isMapValue([1, 2])).toBe(false);
    expect(isMapValue('str')).toBe(false);
    expect(isMapValue(7)).toBe(false);
  });
});

describe('mapSections', () => {
  it('keeps only non-empty maps, sorts them, and stringifies values', () => {
    const sections = mapSections({
      statistics: { rx_packets: 7 },
      external_ids: { a: 'b' },
      other_config: null,
      bfd: {},
      name: 'br-int',
      link_state: 'up',
    });

    expect(sections.map((s) => s.key)).toEqual(['external_ids', 'statistics']);
    // Numeric statistics values are stringified so a KeyValueTable can render
    // them; null and empty maps are dropped, scalars are ignored.
    expect(sections[1].data).toEqual({ rx_packets: '7' });
  });

  it('returns an empty list when the row has no map fields', () => {
    expect(mapSections({ name: 'eth0', mtu: 1500 })).toEqual([]);
  });

  it('sorts the keys within a section', () => {
    const [section] = mapSections({ status: { driver: 'veth', speed: '10' } });
    expect(Object.keys(section.data)).toEqual(['driver', 'speed']);
  });
});

describe('formatLinkSpeed', () => {
  it('formats known magnitudes', () => {
    expect(formatLinkSpeed(10000000000)).toBe('10 Gbps');
    expect(formatLinkSpeed(1000000)).toBe('1 Mbps');
    expect(formatLinkSpeed(100000)).toBe('100 Kbps');
  });

  it('returns a dash for absent or zero speeds', () => {
    expect(formatLinkSpeed(0)).toBe('-');
    expect(formatLinkSpeed(null)).toBe('-');
    expect(formatLinkSpeed(undefined)).toBe('-');
  });
});

describe('linkStateVariant', () => {
  it('maps up/down and treats anything else as neutral', () => {
    expect(linkStateVariant('up')).toBe('success');
    expect(linkStateVariant('down')).toBe('error');
    expect(linkStateVariant(undefined)).toBe('neutral');
    expect(linkStateVariant('unknown')).toBe('neutral');
  });
});

describe('counterVariant', () => {
  it('flags positive counters as errors and zero as neutral', () => {
    expect(counterVariant(0)).toBe('neutral');
    expect(counterVariant(3)).toBe('error');
  });
});

describe('interfaceSignals', () => {
  it('surfaces link/error signals and per-counter variants', () => {
    const signals = interfaceSignals({
      link_state: 'down',
      error: 'no carrier',
      statistics: { rx_errors: 3, tx_dropped: 0 },
    });
    const byLabel = Object.fromEntries(signals.map((s) => [s.label, s]));

    expect(byLabel.link_state).toMatchObject({
      value: 'down',
      variant: 'error',
    });
    expect(byLabel.error).toMatchObject({
      value: 'no carrier',
      variant: 'error',
    });
    expect(byLabel.rx_errors).toMatchObject({ value: 3, variant: 'error' });
    // A zero counter stays neutral; a counter absent from statistics defaults
    // to 0 and is still shown so the tile grid is stable across drivers.
    expect(byLabel.tx_dropped).toMatchObject({ value: 0, variant: 'neutral' });
    expect(byLabel.tx_errors).toMatchObject({ value: 0, variant: 'neutral' });
  });

  it('falls back to dashes and a healthy error tile when fields are absent', () => {
    const signals = interfaceSignals({});
    const byLabel = Object.fromEntries(signals.map((s) => [s.label, s]));

    expect(byLabel.link_state).toMatchObject({
      value: '-',
      variant: 'neutral',
    });
    expect(byLabel.speed.value).toBe('-');
    expect(byLabel.mtu.value).toBe('-');
    expect(byLabel.ofport.value).toBe('-');
    expect(byLabel.mac_in_use.value).toBe('-');
    expect(byLabel.error).toMatchObject({ value: 'none', variant: 'success' });
  });
});
