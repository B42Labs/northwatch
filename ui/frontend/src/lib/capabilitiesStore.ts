import { writable, derived } from 'svelte/store';
import { getCapabilities, type SnapshotInfo } from './api';

/** List of active capabilities fetched from the backend. */
export const capabilities = writable<string[]>([]);

/** Data-source mode: 'live' for a real OVN deployment, 'snapshot' for an offline copy. */
export const appMode = writable<'live' | 'snapshot'>('live');

/** Metadata about the snapshot backing the server, when in snapshot mode. */
export const snapshotInfo = writable<SnapshotInfo | null>(null);

/** Whether the backend has write operations enabled. */
export const writeEnabled = derived(capabilities, ($caps) =>
  $caps.includes('write'),
);

/** Whether the server is serving an offline snapshot instead of live OVN. */
export const snapshotMode = derived(appMode, ($mode) => $mode === 'snapshot');

/** Fetches capabilities from the backend and updates the stores. */
export async function loadCapabilities(): Promise<void> {
  try {
    const info = await getCapabilities();
    capabilities.set(info.capabilities);
    appMode.set(info.mode);
    snapshotInfo.set(info.snapshot ?? null);
  } catch {
    // Silently ignore — capabilities remain empty
  }
}
