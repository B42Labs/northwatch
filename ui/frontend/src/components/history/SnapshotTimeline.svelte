<script lang="ts">
  import type { SnapshotMeta } from '../../lib/api';

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
      class="flex items-start gap-3 rounded-lg border border-base-300 bg-base-100 p-3 shadow-sm"
    >
      <input
        type="checkbox"
        checked={selectedIds.has(snap.id)}
        onchange={() => onToggle(snap.id)}
        class="checkbox checkbox-sm mt-1"
      />
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span
            class="badge badge-sm {snap.trigger === 'auto'
              ? 'badge-ghost'
              : 'badge-primary'}"
          >
            {snap.trigger}
          </span>
          <span class="text-sm font-medium">
            {formatTime(snap.timestamp)}
          </span>
          {#if snap.label}
            <span class="text-sm text-base-content/60">— {snap.label}</span>
          {/if}
        </div>
        <div class="mt-1 flex gap-3 text-xs text-base-content/50">
          <span>{totalRows(snap.row_counts)} rows</span>
          <span>{formatSize(snap.size_bytes)}</span>
          <span>{Object.keys(snap.row_counts).length} tables</span>
        </div>
      </div>
      <div class="flex gap-1">
        <button class="btn btn-ghost btn-xs" onclick={() => onView(snap.id)}>
          View
        </button>
        <button
          class="btn btn-ghost btn-xs text-error"
          onclick={() => onDelete(snap.id)}
        >
          Delete
        </button>
      </div>
    </div>
  {/each}

  {#if snapshots.length === 0}
    <div class="py-8 text-center text-sm text-base-content/40">
      No snapshots yet. Take one to get started.
    </div>
  {/if}
</div>
