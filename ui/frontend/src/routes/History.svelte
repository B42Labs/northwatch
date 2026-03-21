<script lang="ts">
  import { onMount } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import {
    listSnapshots,
    createSnapshot,
    deleteSnapshot,
    diffSnapshots,
    type SnapshotMeta,
    type DiffResult,
  } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import SnapshotTimeline from '../components/history/SnapshotTimeline.svelte';
  import SnapshotViewer from '../components/history/SnapshotViewer.svelte';
  import DiffView from '../components/history/DiffView.svelte';

  let snapshots: SnapshotMeta[] = $state([]);
  let loading = $state(true);
  let error = $state('');
  let creating = $state(false);

  // Snapshot selection for diff
  let selectedIds = new SvelteSet<number>();
  let diff: DiffResult | null = $state(null);
  let diffLoading = $state(false);

  // Viewing a single snapshot
  let viewingSnapshot: SnapshotMeta | null = $state(null);

  async function loadSnapshots() {
    loading = true;
    error = '';
    try {
      snapshots = await listSnapshots();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load snapshots';
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    creating = true;
    error = '';
    try {
      await createSnapshot();
      await loadSnapshots();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create snapshot';
    } finally {
      creating = false;
    }
  }

  async function handleDelete(id: number) {
    error = '';
    try {
      await deleteSnapshot(id);
      selectedIds.delete(id);
      if (viewingSnapshot?.id === id) viewingSnapshot = null;
      if (diff && (diff.from_id === id || diff.to_id === id)) diff = null;
      await loadSnapshots();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete snapshot';
    }
  }

  function handleToggle(id: number) {
    if (selectedIds.has(id)) {
      selectedIds.delete(id);
    } else {
      if (selectedIds.size >= 2) {
        // Replace oldest selection
        const first = selectedIds.values().next().value;
        if (first !== undefined) selectedIds.delete(first);
      }
      selectedIds.add(id);
    }
  }

  function handleView(id: number) {
    const snap = snapshots.find((s) => s.id === id);
    if (snap) {
      viewingSnapshot = snap;
      diff = null;
    }
  }

  async function handleCompare() {
    if (selectedIds.size !== 2) return;
    const ids = [...selectedIds].sort((a, b) => a - b);
    diffLoading = true;
    error = '';
    try {
      diff = await diffSnapshots(ids[0], ids[1]);
      viewingSnapshot = null;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to diff snapshots';
    } finally {
      diffLoading = false;
    }
  }

  onMount(loadSnapshots);
</script>

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Snapshots</h1>
    <p class="text-sm text-base-content/60">
      Browse OVN state snapshots and compare changes
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {/if}

  {#if viewingSnapshot}
    <div class="mb-3">
      <button
        class="btn btn-ghost btn-sm"
        onclick={() => (viewingSnapshot = null)}
      >
        &larr; Back to snapshots
      </button>
    </div>
    <SnapshotViewer
      snapshot={viewingSnapshot}
      onClose={() => (viewingSnapshot = null)}
    />
  {:else if diff}
    <div class="mb-3">
      <button class="btn btn-ghost btn-sm" onclick={() => (diff = null)}>
        &larr; Back to snapshots
      </button>
    </div>
    <DiffView {diff} />
  {:else}
    <div class="mb-3 flex items-center gap-2">
      <button
        class="btn btn-primary btn-sm"
        onclick={handleCreate}
        disabled={creating}
      >
        {creating ? 'Creating...' : 'Take Snapshot'}
      </button>

      {#if selectedIds.size === 2}
        <button
          class="btn btn-outline btn-sm"
          onclick={handleCompare}
          disabled={diffLoading}
        >
          {diffLoading ? 'Comparing...' : 'Compare Selected'}
        </button>
      {:else if selectedIds.size > 0}
        <span class="text-xs text-base-content/50">
          Select 2 snapshots to compare
        </span>
      {/if}

      <button class="btn btn-ghost btn-xs ml-auto" onclick={loadSnapshots}>
        Refresh
      </button>
    </div>

    {#if loading}
      <LoadingSpinner />
    {:else}
      <SnapshotTimeline
        {snapshots}
        {selectedIds}
        onToggle={handleToggle}
        onView={handleView}
        onDelete={handleDelete}
      />
    {/if}
  {/if}
</div>
