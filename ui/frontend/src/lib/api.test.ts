import { describe, it, expect, vi, afterEach } from 'vitest';
import { getTopology, listOvsMembers, listOvsTable } from './api';

function mockFetch(body: unknown): void {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: true,
      json: async () => body,
    })),
  );
}

describe('getTopology', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('normalizes null nodes/edges to empty arrays', async () => {
    // The backend marshals empty (nil) slices to JSON null when OVN has no
    // logical switches/routers provisioned.
    mockFetch({ nodes: null, edges: null });

    const result = await getTopology();

    expect(result).toEqual({ nodes: [], edges: [] });
  });

  it('normalizes missing nodes/edges keys to empty arrays', async () => {
    mockFetch({});

    const result = await getTopology();

    expect(result).toEqual({ nodes: [], edges: [] });
  });

  it('passes through populated nodes/edges unchanged', async () => {
    const body = {
      nodes: [{ id: 'a', type: 'switch', label: 'ls0' }],
      edges: [{ source: 'a', target: 'b', type: 'binding' }],
    };
    mockFetch(body);

    const result = await getTopology();

    expect(result).toEqual(body);
  });

  it('requests VM ports when the vms option is set', async () => {
    const fetchSpy = vi.fn(async () => ({
      ok: true,
      json: async () => ({ nodes: [], edges: [] }),
    }));
    vi.stubGlobal('fetch', fetchSpy);

    await getTopology({ vms: true });

    expect(fetchSpy).toHaveBeenCalledWith('/api/v1/topology?vms=true');
  });

  it('omits the vms query parameter by default', async () => {
    const fetchSpy = vi.fn(async () => ({
      ok: true,
      json: async () => ({ nodes: [], edges: [] }),
    }));
    vi.stubGlobal('fetch', fetchSpy);

    await getTopology();

    expect(fetchSpy).toHaveBeenCalledWith('/api/v1/topology');
  });
});

describe('OVS visibility endpoints', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('lists members against the global mux and normalizes null', async () => {
    const fetchSpy = vi.fn(async () => ({
      ok: true,
      json: async () => null,
    }));
    vi.stubGlobal('fetch', fetchSpy);

    const result = await listOvsMembers();

    expect(fetchSpy).toHaveBeenCalledWith('/api/v1/ovs');
    expect(result).toEqual([]);
  });

  it('passes through populated members', async () => {
    const body = [
      { system_id: 'testbed-node-0', connected: true },
      { system_id: 'testbed-node-1', connected: false },
    ];
    mockFetch(body);

    expect(await listOvsMembers()).toEqual(body);
  });

  it('encodes the chassis and table path segments', async () => {
    const fetchSpy = vi.fn(async () => ({
      ok: true,
      json: async () => [],
    }));
    vi.stubGlobal('fetch', fetchSpy);

    await listOvsTable('node/0', 'interface');

    expect(fetchSpy).toHaveBeenCalledWith('/api/v1/ovs/node%2F0/interface');
  });

  it('normalizes a null table body to an empty array', async () => {
    mockFetch(null);

    expect(await listOvsTable('node-0', 'bridge')).toEqual([]);
  });
});
