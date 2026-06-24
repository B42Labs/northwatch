import { writable, derived } from 'svelte/store';
import { getCapabilities, type SnapshotInfo } from './api';
import { activeClusterInfo } from './clusterStore';

/** List of active capabilities fetched from the backend. */
export const capabilities = writable<string[]>([]);

// baseMode / baseSnapshotInfo describe the default cluster, as reported by
// /api/v1/capabilities. They are the baseline; the active cluster can override
// them — a snapshot loaded at runtime is its own cluster reporting snapshot mode.
const baseMode = writable<'live' | 'snapshot'>('live');
const baseSnapshotInfo = writable<SnapshotInfo | null>(null);

/** Whether the backend has write operations enabled. */
export const writeEnabled = derived(capabilities, ($caps) =>
  $caps.includes('write'),
);

/** Data-source mode of the ACTIVE cluster: 'live' or 'snapshot'. */
export const appMode = derived(
  [baseMode, activeClusterInfo],
  ([$base, $active]): 'live' | 'snapshot' =>
    $active?.mode === 'snapshot' ? 'snapshot' : $base,
);

/** Snapshot metadata for the active cluster, when it is in snapshot mode. */
export const snapshotInfo = derived(
  [baseSnapshotInfo, activeClusterInfo],
  ([$base, $active]): SnapshotInfo | null => {
    if ($active?.mode === 'snapshot') {
      return $active.snapshot
        ? {
            createdAt: $active.snapshot.createdAt,
            nbAddr: $active.snapshot.nbAddr,
            sbAddr: $active.snapshot.sbAddr,
          }
        : null;
    }
    return $base;
  },
);

/** Whether the active data source is an offline snapshot instead of live OVN. */
export const snapshotMode = derived(appMode, ($mode) => $mode === 'snapshot');

/** Fetches capabilities from the backend and updates the stores. */
export async function loadCapabilities(): Promise<void> {
  try {
    const info = await getCapabilities();
    capabilities.set(info.capabilities);
    baseMode.set(info.mode);
    baseSnapshotInfo.set(info.snapshot ?? null);
  } catch {
    // Silently ignore — capabilities remain empty
  }
}
