<script lang="ts">
  import type { FlowTableGroup } from '../../lib/api';
  import FlowTable from './FlowTable.svelte';

  let { label, tables }: { label: string; tables: FlowTableGroup[] } = $props();
</script>

<div>
  <h3
    class="mb-2 text-sm font-semibold uppercase tracking-wide text-base-content/60"
  >
    {label} Pipeline
  </h3>
  {#if tables.length === 0}
    <div class="py-4 text-center text-sm text-base-content/40">
      No flows in this pipeline
    </div>
  {:else}
    <div class="flex gap-3 overflow-x-auto pb-2">
      {#each tables as tbl, i}
        <div class="flex items-start gap-2">
          <FlowTable tableId={tbl.table_id} flows={tbl.flows} />
          {#if i < tables.length - 1}
            <div class="flex items-center self-center text-base-content/30">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-5 w-5"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fill-rule="evenodd"
                  d="M10.293 3.293a1 1 0 011.414 0l6 6a1 1 0 010 1.414l-6 6a1 1 0 01-1.414-1.414L14.586 11H3a1 1 0 110-2h11.586l-4.293-4.293a1 1 0 010-1.414z"
                  clip-rule="evenodd"
                />
              </svg>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
