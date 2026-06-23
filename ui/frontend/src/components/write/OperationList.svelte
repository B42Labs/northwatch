<script lang="ts">
  import type { WriteOperation } from '../../lib/writeApi';
  import Badge from '../ui/Badge.svelte';
  import { actionVariant, actionGlyph } from '../../lib/status';

  let {
    operations,
    onRemove,
  }: {
    operations: WriteOperation[];
    onRemove: (index: number) => void;
  } = $props();
</script>

{#if operations.length === 0}
  <div class="py-4 text-center font-mono text-sm text-base-content/40">
    <span class="text-base-content/30">//</span> no operations added yet — use the
    form above to add operations
  </div>
{:else}
  <div class="flex flex-col gap-2">
    {#each operations as op, i (i)}
      <div
        class="flex items-center justify-between rounded border border-base-300 bg-base-100 px-3 py-2"
      >
        <div class="flex items-center gap-2">
          <Badge
            text={op.action}
            variant={actionVariant(op.action)}
            glyph={actionGlyph(op.action)}
          />
          <span class="font-mono text-sm">{op.table}</span>
          {#if op.uuid}
            <span class="font-mono text-xs text-base-content/50">
              {op.uuid.slice(0, 8)}
            </span>
          {/if}
          {#if op.fields}
            <span class="font-mono text-xs text-base-content/50">
              {Object.keys(op.fields).length} field(s)
            </span>
          {/if}
          {#if op.reason}
            <span class="font-mono text-xs italic text-base-content/40">
              {op.reason}
            </span>
          {/if}
        </div>
        <button
          class="btn btn-ghost btn-xs border-base-300 font-mono text-error"
          onclick={() => onRemove(i)}
        >
          Remove
        </button>
      </div>
    {/each}
  </div>
{/if}
