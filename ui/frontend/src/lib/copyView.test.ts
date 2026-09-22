import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import {
  copyText,
  maskEnabled,
  maskSensitive,
  renderJSON,
  renderMarkdown,
  viewContext,
  type ViewContext,
} from './copyView';
import { location as routeLocation } from './router';
import { clusters, activeCluster } from './clusterStore';

const START = new Date('2026-09-22T08:30:00.000Z').getTime();

function ctx(overrides: Partial<ViewContext> = {}): ViewContext {
  return {
    title: 'Port Diagnostics',
    cluster: 'prod',
    clusterLabel: 'Production',
    mode: 'live',
    route: '/debug/port-diagnostics',
    query: { severity: 'error' },
    copiedAt: '2026-09-22T08:30:00.000Z',
    masked: false,
    ...overrides,
  };
}

describe('copyText', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    delete (document as Partial<Document>).execCommand;
  });

  it('uses the async clipboard API in a secure context', async () => {
    const writeText = vi.fn(async () => {});
    const execCommand = vi.fn(() => true);
    vi.stubGlobal('isSecureContext', true);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    document.execCommand = execCommand;

    await copyText('hello');

    expect(writeText).toHaveBeenCalledWith('hello');
    expect(execCommand).not.toHaveBeenCalled();
  });

  it('falls back to execCommand when the clipboard API is absent', async () => {
    const execCommand = vi.fn(() => true);
    vi.stubGlobal('isSecureContext', false);
    vi.stubGlobal('navigator', {});
    document.execCommand = execCommand;

    await copyText('over plain http');

    expect(execCommand).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('falls back to execCommand when writeText rejects', async () => {
    const writeText = vi.fn(async () => {
      throw new DOMException('not focused', 'NotAllowedError');
    });
    const execCommand = vi.fn(() => true);
    vi.stubGlobal('isSecureContext', true);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    document.execCommand = execCommand;

    await copyText('retry me');

    expect(writeText).toHaveBeenCalled();
    expect(execCommand).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('rejects and removes the textarea when execCommand fails', async () => {
    const execCommand = vi.fn(() => false);
    vi.stubGlobal('isSecureContext', false);
    vi.stubGlobal('navigator', {});
    document.execCommand = execCommand;

    await expect(copyText('nope')).rejects.toThrow('clipboard unavailable');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('copies the empty string without erroring', async () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('isSecureContext', true);
    vi.stubGlobal('navigator', { clipboard: { writeText } });

    await copyText('');

    expect(writeText).toHaveBeenCalledWith('');
  });
});

describe('viewContext', () => {
  beforeEach(() => {
    clusters.set([]);
    activeCluster.set('');
    routeLocation.set('/');
    vi.useFakeTimers();
    vi.setSystemTime(START);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('falls back to the default cluster when none is registered', () => {
    const result = viewContext('Raw', false);

    expect(result.cluster).toBe('default');
    expect(result.clusterLabel).toBe('');
    expect(result.mode).toBe('live');
    expect(result.snapshotCreatedAt).toBeUndefined();
  });

  it('names the active cluster', () => {
    clusters.set([{ name: 'prod', label: 'Production', ready: true }]);
    activeCluster.set('prod');

    const result = viewContext('Raw', false);

    expect(result.cluster).toBe('prod');
    expect(result.clusterLabel).toBe('Production');
  });

  it('reports snapshot mode with the snapshot creation time', () => {
    clusters.set([
      {
        name: 'snap-1',
        label: 'Snapshot 1',
        ready: true,
        mode: 'snapshot',
        snapshot: { sourceId: 1, createdAt: '2026-09-01T10:00:00Z' },
      },
    ]);
    activeCluster.set('snap-1');

    const result = viewContext('Raw', false);

    expect(result.mode).toBe('snapshot');
    expect(result.snapshotCreatedAt).toBe('2026-09-01T10:00:00Z');
  });

  it('reports snapshot mode without a time when the snapshot is unknown', () => {
    clusters.set([
      { name: 'snap-2', label: 'Snapshot 2', ready: true, mode: 'snapshot' },
    ]);
    activeCluster.set('snap-2');

    const result = viewContext('Raw', false);

    expect(result.mode).toBe('snapshot');
    expect(result.snapshotCreatedAt).toBeUndefined();
  });

  it('splits the route from its query', () => {
    routeLocation.set('/debug/port-diagnostics?severity=error');

    const result = viewContext('Port Diagnostics', false);

    expect(result.route).toBe('/debug/port-diagnostics');
    expect(result.query).toEqual({ severity: 'error' });
  });

  it('reports an empty query when the route has none', () => {
    routeLocation.set('/ovs');

    const result = viewContext('OVS Visibility', false);

    expect(result.route).toBe('/ovs');
    expect(result.query).toEqual({});
  });

  it('stamps the current time and the mask state', () => {
    const result = viewContext('Raw', true);

    expect(result.copiedAt).toBe(new Date(START).toISOString());
    expect(result.masked).toBe(true);
  });
});

describe('renderJSON', () => {
  it('serializes with two-space indentation and no header', () => {
    expect(renderJSON({ a: 1 })).toBe(JSON.stringify({ a: 1 }, null, 2));
  });

  it('propagates the TypeError from a cyclic object', () => {
    const cyclic: Record<string, unknown> = {};
    cyclic.self = cyclic;

    expect(() => renderJSON(cyclic)).toThrow(TypeError);
  });
});

describe('renderMarkdown', () => {
  it('writes the context header above a fenced json block', () => {
    const data = { total: 3 };
    const out = renderMarkdown(ctx(), data);

    expect(out.startsWith('## Northwatch view: Port Diagnostics')).toBe(true);
    expect(out).toContain('- Cluster: prod (Production)');
    expect(out).toContain('- Mode: live');
    expect(out).toContain('- Route: /debug/port-diagnostics');
    expect(out).toContain('- Query: severity=error');
    expect(out).toContain('- Copied at: 2026-09-22T08:30:00.000Z');
    expect(out).toContain('- Masked: no');
    expect(out).toContain(
      '```json\n' + JSON.stringify(data, null, 2) + '\n```',
    );
  });

  it('omits the parenthesised label when the cluster has none', () => {
    const out = renderMarkdown(
      ctx({ cluster: 'default', clusterLabel: '' }),
      {},
    );

    expect(out).toContain('- Cluster: default\n');
    expect(out).not.toContain('- Cluster: default (');
  });

  it('names the snapshot creation time in snapshot mode', () => {
    const out = renderMarkdown(
      ctx({ mode: 'snapshot', snapshotCreatedAt: '2026-09-01T10:00:00Z' }),
      {},
    );

    expect(out).toContain('- Mode: snapshot (created 2026-09-01T10:00:00Z)');
  });

  it('reports bare snapshot mode when the time is unknown', () => {
    const out = renderMarkdown(ctx({ mode: 'snapshot' }), {});

    expect(out).toContain('- Mode: snapshot\n');
  });

  it('reports an empty query as (none)', () => {
    const out = renderMarkdown(ctx({ query: {} }), {});

    expect(out).toContain('- Query: (none)');
  });

  it('reports the mask state when masking ran', () => {
    const out = renderMarkdown(ctx({ masked: true }), {});

    expect(out).toContain('- Masked: yes');
  });

  it('renders null for absent data', () => {
    expect(renderMarkdown(ctx(), null)).toContain('```json\nnull\n```');
    expect(renderMarkdown(ctx(), undefined)).toContain('```json\nnull\n```');
  });

  it('propagates the TypeError from a cyclic object', () => {
    const cyclic: Record<string, unknown> = {};
    cyclic.self = cyclic;

    expect(() => renderMarkdown(ctx(), cyclic)).toThrow(TypeError);
  });
});

describe('maskSensitive', () => {
  it('passes through everything that carries no address', () => {
    expect(maskSensitive(null)).toBeNull();
    expect(maskSensitive(undefined)).toBeUndefined();
    expect(maskSensitive([])).toEqual([]);
    expect(maskSensitive({})).toEqual({});
    expect(maskSensitive('')).toBe('');
    expect(maskSensitive(42)).toBe(42);
    expect(maskSensitive(true)).toBe(true);
  });

  it('masks IPv4 addresses and keeps the prefix length', () => {
    expect(maskSensitive('10.0.0.5')).toBe('ip-1');
    expect(maskSensitive('10.0.0.5/24')).toBe('ip-1/24');
  });

  it('keeps a trailing port on an IPv4 address', () => {
    expect(maskSensitive('192.168.1.1:6642')).toBe('ip-1:6642');
  });

  it('masks IPv6 addresses', () => {
    expect(maskSensitive('fd00::1/64')).toBe('ip6-1/64');
    expect(maskSensitive('2001:db8::ff00:42:8329')).toBe('ip6-1');
  });

  it('masks a full eight-group IPv6 address', () => {
    expect(maskSensitive('2001:0db8:85a3:0000:0000:8a2e:0370:7334')).toBe(
      'ip6-1',
    );
  });

  it('leaves colon-separated values that are not addresses alone', () => {
    expect(maskSensitive('10:00:00')).toBe('10:00:00');
    expect(maskSensitive('2026-09-01T10:00:00Z')).toBe('2026-09-01T10:00:00Z');
    expect(maskSensitive('tcp:6642')).toBe('tcp:6642');
  });

  it('masks MAC addresses before IPv6 can claim them', () => {
    expect(maskSensitive('0a:00:00:00:00:01 10.0.0.5')).toBe('mac-1 ip-1');
    expect(maskSensitive('0A:00:00:00:00:01')).toBe('mac-1');
  });

  it('gives one placeholder per distinct address', () => {
    expect(
      maskSensitive({ a: '10.0.0.5', b: '10.0.0.5', c: '10.0.0.6' }),
    ).toEqual({ a: 'ip-1', b: 'ip-1', c: 'ip-2' });
  });

  it('masks a hostname value only under a hostname key', () => {
    expect(maskSensitive({ chassis: { hostname: 'node-1.lab' } })).toEqual({
      chassis: { hostname: 'host-1' },
    });
    expect(maskSensitive({ name: 'node-1.lab' })).toEqual({
      name: 'node-1.lab',
    });
  });

  it('never rewrites object keys', () => {
    expect(maskSensitive({ '10.0.0.5': 'x' })).toEqual({ '10.0.0.5': 'x' });
  });

  it('walks arrays element by element', () => {
    expect(maskSensitive(['10.0.0.5', '10.0.0.6'])).toEqual(['ip-1', 'ip-2']);
  });

  it('leaves the input untouched', () => {
    const input = { addresses: ['0a:00:00:00:00:01 10.0.0.5'] };
    const before = structuredClone(input);

    maskSensitive(input);

    expect(input).toEqual(before);
  });
});

describe('maskEnabled', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('defaults to off when nothing is stored', async () => {
    const mod = await import('./copyView');

    expect(get(mod.maskEnabled)).toBe(false);
  });

  it('reads a stored opt-in', async () => {
    localStorage.setItem('northwatch-copy-mask', '1');

    const mod = await import('./copyView');

    expect(get(mod.maskEnabled)).toBe(true);
  });

  it('ignores a stored value it does not recognise', async () => {
    localStorage.setItem('northwatch-copy-mask', 'yes');

    const mod = await import('./copyView');

    expect(get(mod.maskEnabled)).toBe(false);
  });

  it('persists the choice', () => {
    maskEnabled.set(true);
    expect(localStorage.getItem('northwatch-copy-mask')).toBe('1');

    maskEnabled.set(false);
    expect(localStorage.getItem('northwatch-copy-mask')).toBe('0');
  });
});
