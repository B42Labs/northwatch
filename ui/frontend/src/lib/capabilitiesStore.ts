import { writable, derived } from 'svelte/store';
import { getCapabilities } from './api';

/** List of active capabilities fetched from the backend. */
export const capabilities = writable<string[]>([]);

/** Whether the backend has write operations enabled. */
export const writeEnabled = derived(capabilities, ($caps) =>
  $caps.includes('write'),
);

/** Fetches capabilities from the backend and updates the store. */
export async function loadCapabilities(): Promise<void> {
  try {
    const caps = await getCapabilities();
    capabilities.set(caps);
  } catch {
    // Silently ignore — capabilities remain empty
  }
}
