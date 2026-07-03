<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import { SvelteSet } from 'svelte/reactivity';
  import {
    listSnapshots,
    createSnapshot,
    deleteSnapshot,
    diffSnapshots,
    loadSnapshot,
    unloadSnapshot,
    type SnapshotMeta,
    type DiffResult,
  } from '../lib/api';
  import {
    loadClusters,
    activeCluster,
    loadedSnapshots,
    firstLiveClusterName,
  } from '../lib/clusterStore';
  import { push } from '../lib/router';
  import PageContainer from '../components/ui/PageContainer.svelte';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import LoadingOverlay from '../components/ui/LoadingOverlay.svelte';
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

  // Load/eject can take a while (the backend materializes the snapshot or
  // reloads live tables). Track which snapshot is busy so we can show a blocking
  // overlay and prevent repeated clicks.
  let busyId: number | null = $state(null);
  let busyLabel = $state('');

  // IDs of snapshots currently loaded as data sources, derived from the cluster
  // list so the timeline stays in sync as snapshots are loaded/unloaded.
  let loadedIds = $derived(
    new SvelteSet(
      $loadedSnapshots
        .map((c) => c.snapshot?.sourceId)
        .filter((id): id is number => id !== undefined),
    ),
  );

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

  // Load a stored snapshot as a read-only data source and switch the whole UI to
  // it. Navigating home remounts the views so they fetch the snapshot's state.
  async function handleLoad(id: number) {
    if (busyId !== null) return; // already loading/ejecting something
    error = '';
    busyId = id;
    busyLabel = 'loading snapshot…';
    try {
      const res = await loadSnapshot(id);
      await loadClusters();
      activeCluster.set(res.cluster);
      push('/');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load snapshot';
    } finally {
      busyId = null;
    }
  }

  async function handleUnload(id: number) {
    if (busyId !== null) return;
    error = '';
    busyId = id;
    busyLabel = 'ejecting snapshot…';
    const wasActive = get(activeCluster) === `snapshot-${id}`;
    try {
      // Unload first: the backend resumes the live OVN connection and reloads
      // its tables. Only then switch the UI to live, so it shows fresh data.
      await unloadSnapshot(id);
      await loadClusters();
      if (wasActive) activeCluster.set(firstLiveClusterName());
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to unload snapshot';
    } finally {
      busyId = null;
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

<PageContainer width="wide">
  <PageHeader
    eyebrow="History"
    title="Snapshots"
    description="Browse OVN state snapshots and compare changes"
  />

  {#if error}
    <div class="mb-3">
      <ErrorAlert message={error} />
    </div>
  {/if}

  {#if viewingSnapshot}
    <div class="mb-3">
      <button
        class="btn border-base-300 btn-ghost btn-sm"
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
      <button
        class="btn border-base-300 btn-ghost btn-sm"
        onclick={() => (diff = null)}
      >
        &larr; Back to snapshots
      </button>
    </div>
    <DiffView {diff} />
  {:else}
    <div class="mb-3 flex items-center gap-2">
      <button
        class="btn font-mono btn-primary btn-sm"
        onclick={handleCreate}
        disabled={creating}
      >
        {creating ? 'Creating...' : 'Take Snapshot'}
      </button>

      {#if selectedIds.size === 2}
        <button
          class="btn btn-outline font-mono btn-sm"
          onclick={handleCompare}
          disabled={diffLoading}
        >
          {diffLoading ? 'Comparing...' : 'Compare Selected'}
        </button>
      {:else if selectedIds.size > 0}
        <span class="font-mono text-xs text-base-content/50">
          Select 2 snapshots to compare
        </span>
      {/if}

      <button
        class="btn ml-auto border-base-300 btn-ghost btn-xs"
        onclick={loadSnapshots}
      >
        Refresh
      </button>
    </div>

    <DataState {loading}>
      <SnapshotTimeline
        {snapshots}
        {selectedIds}
        {loadedIds}
        {busyId}
        onToggle={handleToggle}
        onView={handleView}
        onLoad={handleLoad}
        onUnload={handleUnload}
        onDelete={handleDelete}
      />
    </DataState>
  {/if}
</PageContainer>

<LoadingOverlay show={busyId !== null} message={busyLabel} />
