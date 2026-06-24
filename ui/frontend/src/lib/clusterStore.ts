import { writable, derived, get } from 'svelte/store';

export interface ClusterSnapshotInfo {
  sourceId: number;
  createdAt?: string;
  nbAddr?: string;
  sbAddr?: string;
}

export interface ClusterInfo {
  name: string;
  label: string;
  ready: boolean;
  /** 'live' for a real OVN deployment, 'snapshot' for a loaded history snapshot. */
  mode?: 'live' | 'snapshot';
  /** Present when mode === 'snapshot': which history snapshot backs this cluster. */
  snapshot?: ClusterSnapshotInfo;
}

/** List of all available clusters, fetched from /api/v1/clusters. */
export const clusters = writable<ClusterInfo[]>([]);

/** The currently selected cluster name, or empty string for the default. */
export const activeCluster = writable<string>('');

/** Whether more than one cluster is selectable (multiple OVN clusters, or a
 * live cluster plus one or more loaded snapshots). Drives the cluster switcher. */
export const multiClusterEnabled = derived(
  clusters,
  ($clusters) => $clusters.length > 1,
);

/** The full info for the currently selected cluster, or null if not found. */
export const activeClusterInfo = derived(
  [clusters, activeCluster],
  ([$clusters, $active]) => {
    const name = $active === '' ? 'default' : $active;
    return $clusters.find((c) => c.name === name) ?? null;
  },
);

/** Loaded snapshot clusters (mode === 'snapshot'), in registration order. */
export const loadedSnapshots = derived(clusters, ($clusters) =>
  $clusters.filter((c) => c.mode === 'snapshot'),
);

/**
 * Name of the first live (non-snapshot) cluster, used as the target when
 * switching away from a snapshot. Falls back to 'default' (the main mux) when no
 * live cluster is listed yet.
 */
export function firstLiveClusterName(): string {
  const live = get(clusters).find((c) => c.mode !== 'snapshot');
  return live?.name ?? 'default';
}

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
