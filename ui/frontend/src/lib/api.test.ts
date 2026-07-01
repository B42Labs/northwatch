import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  ApiError,
  getOvsEntity,
  getOvsInterfaceCorrelation,
  getTopology,
  listOvsMembers,
  listOvsTable,
} from './api';

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

  it('gets a single entity with encoded path segments via the global mux', async () => {
    const body = { _uuid: 'uuid-1', name: 'br-int' };
    const fetchSpy = vi.fn(async () => ({
      ok: true,
      json: async () => body,
    }));
    vi.stubGlobal('fetch', fetchSpy);

    const result = await getOvsEntity('node/0', 'interface', 'uuid-1');

    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/v1/ovs/node%2F0/interface/uuid-1',
    );
    expect(result).toEqual(body);
  });

  it('surfaces a missing entity as an ApiError', async () => {
    const fetchSpy = vi.fn(async () => ({
      ok: false,
      status: 404,
      statusText: 'Not Found',
      json: async () => ({ error: 'not found' }),
    }));
    vi.stubGlobal('fetch', fetchSpy);

    const err = await getOvsEntity('node-0', 'interface', 'missing').catch(
      (e) => e,
    );
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 404, message: 'not found' });
  });

  it('correlates an interface via the global mux with encoded path segments', async () => {
    const body = { iface_id: 'lsp-a', bound: false };
    const fetchSpy = vi.fn(async () => ({
      ok: true,
      json: async () => body,
    }));
    vi.stubGlobal('fetch', fetchSpy);

    const result = await getOvsInterfaceCorrelation('node/0', 'uuid-1');

    expect(fetchSpy).toHaveBeenCalledWith(
      '/api/v1/ovs/node%2F0/interface/uuid-1/correlation',
    );
    expect(result).toEqual(body);
  });
});
