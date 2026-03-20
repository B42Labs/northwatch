<script lang="ts">
  import type { FlowEntry } from '../../lib/api';
  import { SvelteSet } from 'svelte/reactivity';

  let { tableId, flows }: { tableId: number; flows: FlowEntry[] } = $props();

  let expandedFlows = new SvelteSet<string>();

  function toggleFlow(uuid: string) {
    if (expandedFlows.has(uuid)) {
      expandedFlows.delete(uuid);
    } else {
      expandedFlows.add(uuid);
    }
  }
</script>

<div class="min-w-[280px] rounded-lg border border-base-300 bg-base-100">
  <div class="border-b border-base-300 bg-base-200 px-3 py-2">
    <span class="text-sm font-semibold">Table {tableId}</span>
    <span class="ml-1 text-xs text-base-content/50">({flows.length} flows)</span
    >
  </div>
  <div class="max-h-[400px] overflow-y-auto">
    {#each flows as flow}
      <button
        type="button"
        class="block w-full cursor-pointer border-b border-base-300 px-3 py-1.5 text-left text-xs last:border-0 hover:bg-base-200"
        onclick={() => toggleFlow(flow.uuid)}
      >
        <div class="flex items-center gap-2">
          <span class="badge badge-ghost badge-xs font-mono"
            >{flow.priority}</span
          >
          <span class="truncate font-mono text-base-content/80"
            >{flow.match || '(empty)'}</span
          >
        </div>
        {#if expandedFlows.has(flow.uuid)}
          <div class="mt-1.5 space-y-1 rounded bg-base-200 p-2">
            <div>
              <span class="font-semibold text-base-content/60">Match:</span>
              <span class="break-all font-mono">{flow.match || '(empty)'}</span>
            </div>
            <div>
              <span class="font-semibold text-base-content/60">Actions:</span>
              <span class="break-all font-mono">{flow.actions}</span>
            </div>
            <div class="text-base-content/40">UUID: {flow.uuid}</div>
          </div>
        {/if}
      </button>
    {/each}
  </div>
</div>
