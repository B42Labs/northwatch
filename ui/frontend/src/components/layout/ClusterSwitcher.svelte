<script lang="ts">
  import {
    clusters,
    activeCluster,
    activeClusterInfo,
    multiClusterEnabled,
    clustersError,
    loadClusters,
    firstLiveClusterName,
  } from '../../lib/clusterStore';
  import { unloadSnapshot } from '../../lib/api';
  import LoadingOverlay from '../ui/LoadingOverlay.svelte';

  let snapshotActive = $derived($activeClusterInfo?.mode === 'snapshot');

  // Ejecting resumes the live OVN connection and reloads its tables, which takes
  // a moment; block with an overlay and guard against repeated clicks.
  let busy = $state(false);

  // Eject the active snapshot: unload first (the backend resumes the live OVN
  // connection and reloads its tables), then refresh the list and switch the UI
  // to live so it shows fresh data rather than the suspended/frozen cache.
  async function eject() {
    if (busy) return;
    const info = $activeClusterInfo;
    if (!info?.snapshot) return;
    const id = info.snapshot.sourceId;
    busy = true;
    try {
      await unloadSnapshot(id);
    } finally {
      await loadClusters();
      activeCluster.set(firstLiveClusterName());
      busy = false;
    }
  }
</script>

{#if $clustersError}
  <span
    class="badge badge-error badge-outline badge-xs gap-1 font-mono text-2xs uppercase tracking-wider"
    title={$clustersError}
  >
    sources unavailable
  </span>
{/if}

{#if $multiClusterEnabled}
  <label class="flex items-center gap-1.5">
    <span
      class="hidden font-mono text-2xs uppercase tracking-wider text-base-content/45 sm:inline"
      >source</span
    >
    <select
      class="select select-bordered select-xs bg-base-200/60 font-mono"
      value={$activeCluster}
      onchange={(e) => activeCluster.set((e.target as HTMLSelectElement).value)}
      disabled={busy}
      aria-label="Active data source"
    >
      {#each $clusters as c (c.name)}
        <option value={c.name}>
          {c.mode === 'snapshot' ? '◴ ' : ''}{c.label}{c.ready
            ? ''
            : ' (offline)'}
        </option>
      {/each}
    </select>
  </label>
  {#if snapshotActive}
    <button
      class="btn btn-ghost btn-xs border-base-300"
      onclick={eject}
      disabled={busy}
      title="Unload snapshot and return to live"
    >
      {#if busy}
        <span class="loading loading-spinner loading-xs"></span>
      {/if}
      eject
    </button>
  {/if}
{/if}

<LoadingOverlay
  show={busy}
  message="ejecting snapshot — reloading live tables…"
/>
