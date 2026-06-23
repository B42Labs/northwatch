<script lang="ts">
  import type { SnapshotMeta } from '../../lib/api';
  import Badge from '../ui/Badge.svelte';
  import EmptyState from '../ui/EmptyState.svelte';

  interface Props {
    snapshots: SnapshotMeta[];
    selectedIds: Set<number>;
    onToggle: (id: number) => void;
    onView: (id: number) => void;
    onDelete: (id: number) => void;
  }

  let { snapshots, selectedIds, onToggle, onView, onDelete }: Props = $props();

  function formatTime(ts: string): string {
    return new Date(ts).toLocaleString();
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  function totalRows(counts: Record<string, number>): number {
    return Object.values(counts).reduce((a, b) => a + b, 0);
  }
</script>

<div class="flex flex-col gap-2">
  {#each snapshots as snap (snap.id)}
    <div
      class="flex items-start gap-3 rounded border border-base-300 bg-base-100 p-3"
    >
      <input
        type="checkbox"
        checked={selectedIds.has(snap.id)}
        onchange={() => onToggle(snap.id)}
        class="checkbox checkbox-sm mt-1"
      />
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <Badge
            text={snap.trigger}
            variant={snap.trigger === 'auto' ? 'neutral' : 'primary'}
          />
          <span class="font-mono text-sm font-medium">
            {formatTime(snap.timestamp)}
          </span>
          {#if snap.label}
            <span class="font-mono text-sm text-base-content/60"
              >— {snap.label}</span
            >
          {/if}
        </div>
        <div class="mt-1 flex gap-3 font-mono text-xs text-base-content/55">
          <span>{totalRows(snap.row_counts)} rows</span>
          <span>{formatSize(snap.size_bytes)}</span>
          <span>{Object.keys(snap.row_counts).length} tables</span>
        </div>
      </div>
      <div class="flex gap-1">
        <button
          class="btn btn-ghost btn-xs border-base-300"
          onclick={() => onView(snap.id)}
        >
          View
        </button>
        <button
          class="btn btn-ghost btn-xs border-base-300 text-error"
          onclick={() => onDelete(snap.id)}
        >
          Delete
        </button>
      </div>
    </div>
  {/each}

  {#if snapshots.length === 0}
    <EmptyState
      message="no snapshots yet"
      hint="Take one to get started."
    />
  {/if}
</div>
