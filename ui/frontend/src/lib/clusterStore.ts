import { writable, derived, get } from 'svelte/store';

export interface ClusterInfo {
  name: string;
  label: string;
  ready: boolean;
}

/** List of all available clusters, fetched from /api/v1/clusters. */
export const clusters = writable<ClusterInfo[]>([]);

/** The currently selected cluster name, or empty string for the default. */
export const activeCluster = writable<string>('');

/** Whether multi-cluster mode is active (more than one cluster). */
export const multiClusterEnabled = derived(
  clusters,
  ($clusters) => $clusters.length > 1,
);

/** Returns the API base path for the active cluster. */
export const clusterApiPrefix = derived(activeCluster, ($active) => {
  if ($active === '' || $active === 'default') {
    return '';
  }
  return `/api/v1/clusters/${$active}`;
});

/**
 * Rewrites an API path to include the cluster prefix when in multi-cluster mode.
 * E.g. "/api/v1/nb/logical-switches" -> "/api/v1/clusters/prod/nb/logical-switches"
 */
export function clusterPath(path: string): string {
  const prefix = get(clusterApiPrefix);
  if (prefix === '') {
    return path;
  }
  // Strip /api/v1 prefix and prepend the cluster-scoped prefix
  if (path.startsWith('/api/v1/')) {
    return prefix + path.slice('/api/v1'.length);
  }
  return path;
}

/** Fetches the cluster list from the server and updates the store. */
export async function loadClusters(): Promise<void> {
  try {
    const res = await fetch('/api/v1/clusters');
    if (!res.ok) return;
    const data = await res.json();
    const list: ClusterInfo[] = data.clusters || [];
    clusters.set(list);

    // If no cluster is selected and we have clusters, select the first one
    const current = get(activeCluster);
    if (current === '' && list.length > 0) {
      activeCluster.set(list[0].name);
    }
  } catch {
    // Silently ignore fetch errors during startup
  }
}
