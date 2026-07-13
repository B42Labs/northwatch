import { describe, it, expect, vi, afterEach } from 'vitest';
import {
  ApiError,
  createSnapshot,
  getOvsEntity,
  getOvsFleetHealth,
  getOvsInterfaceCorrelation,
  getTopology,
  listOvsMembers,
  listOvsTable,
} from './api';
import { clearApiToken, setApiToken } from './authStore';

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

  it('fetches fleet health from the global mux', async () => {
    const body = {
      chassis: 2,
      connected: 1,
      unreachable: 1,
      bridges: 3,
      ports: 4,
      interfaces: 5,
      down_interfaces: 1,
      error_interfaces: 2,
      members: [{ system_id: 'node-0', connected: true }],
    };
    const fetchSpy = vi.fn(async () => ({
      ok: true,
      json: async () => body,
    }));
    vi.stubGlobal('fetch', fetchSpy);

    const result = await getOvsFleetHealth();

    expect(fetchSpy).toHaveBeenCalledWith('/api/v1/ovs/health');
    expect(result).toEqual(body);
  });

  it('normalizes a null members list to an empty array', async () => {
    mockFetch({ chassis: 0, connected: 0, unreachable: 0, members: null });

    const result = await getOvsFleetHealth();

    expect(result.members).toEqual([]);
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

describe('bearer token', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    clearApiToken();
  });

  it('omits the Authorization header when no token is set', async () => {
    clearApiToken();
    const spy = vi.fn(async () => ({ ok: true, json: async () => ({}) }));
    vi.stubGlobal('fetch', spy);

    await listOvsMembers();

    expect(spy).toHaveBeenCalledWith('/api/v1/ovs');
  });

  it('sends the token as a bearer credential once configured', async () => {
    setApiToken('0123456789abcdef');
    const spy = vi.fn(async () => ({
      ok: true,
      json: async () => ({ id: 1 }),
    }));
    vi.stubGlobal('fetch', spy);

    await createSnapshot('pre-upgrade');

    expect(spy).toHaveBeenCalledWith('/api/v1/snapshots', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: 'Bearer 0123456789abcdef',
      },
      body: JSON.stringify({ label: 'pre-upgrade' }),
    });
  });

  it('turns a 401 into a message that says what to do about it', async () => {
    clearApiToken();
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
        json: async () => ({ error: 'authentication required' }),
      })),
    );

    const err = await createSnapshot().catch((e) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 401,
      message: 'authentication required — set an API token',
    });
  });
});
