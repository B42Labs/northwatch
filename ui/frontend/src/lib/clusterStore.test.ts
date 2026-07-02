import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import {
  clusters,
  activeCluster,
  clustersError,
  loadClusters,
} from './clusterStore';

describe('loadClusters', () => {
  beforeEach(() => {
    clusters.set([]);
    activeCluster.set('');
    clustersError.set(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sets clustersError when the response is not ok', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        status: 503,
        statusText: 'Service Unavailable',
        json: async () => ({}),
      })),
    );

    await loadClusters();

    const err = get(clustersError);
    expect(err).not.toBeNull();
    expect(err).toContain('503');
    expect(err).toContain('Service Unavailable');
  });

  it('sets clustersError when fetch throws', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('network down');
      }),
    );

    await loadClusters();

    expect(get(clustersError)).toBe('network down');
  });

  it('clears clustersError on a successful load', async () => {
    clustersError.set('stale failure');
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        json: async () => ({
          clusters: [{ name: 'default', label: 'Default', ready: true }],
        }),
      })),
    );

    await loadClusters();

    expect(get(clustersError)).toBeNull();
    expect(get(clusters)).toHaveLength(1);
  });
});
