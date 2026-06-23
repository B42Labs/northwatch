import { describe, it, expect, vi, afterEach } from 'vitest';
import { getTopology } from './api';

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
