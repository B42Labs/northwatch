<script lang="ts">
  import type { WriteOperation } from '../../lib/writeApi';
  import Badge from '../ui/Badge.svelte';

  let {
    operations,
    onRemove,
  }: {
    operations: WriteOperation[];
    onRemove: (index: number) => void;
  } = $props();

  function actionVariant(
    action: string,
  ): 'success' | 'warning' | 'error' | 'neutral' {
    switch (action) {
      case 'create':
        return 'success';
      case 'update':
        return 'warning';
      case 'delete':
        return 'error';
      default:
        return 'neutral';
    }
  }
</script>

{#if operations.length === 0}
  <div class="py-4 text-center text-sm text-base-content/50">
    No operations added yet. Use the form above to add operations.
  </div>
{:else}
  <div class="flex flex-col gap-2">
    {#each operations as op, i (i)}
      <div
        class="flex items-center justify-between rounded-lg border border-base-300 bg-base-100 px-3 py-2"
      >
        <div class="flex items-center gap-2">
          <Badge text={op.action} variant={actionVariant(op.action)} />
          <span class="font-mono text-sm">{op.table}</span>
          {#if op.uuid}
            <span class="font-mono text-xs text-base-content/50">
              {op.uuid.slice(0, 8)}
            </span>
          {/if}
          {#if op.fields}
            <span class="text-xs text-base-content/50">
              {Object.keys(op.fields).length} field(s)
            </span>
          {/if}
          {#if op.reason}
            <span class="text-xs italic text-base-content/40">
              {op.reason}
            </span>
          {/if}
        </div>
        <button
          class="btn btn-ghost btn-xs text-error"
          onclick={() => onRemove(i)}
        >
          Remove
        </button>
      </div>
    {/each}
  </div>
{/if}
