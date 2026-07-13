<script lang="ts">
  import type { EventRecord } from '../../lib/api';
  import { actionVariant, actionGlyph } from '../../lib/status';
  import Badge from '../ui/Badge.svelte';

  interface Props {
    event: EventRecord;
    onClose: () => void;
  }

  let { event, onClose }: Props = $props();

  let showRawJson = $state(false);

  function changedKeys(
    oldRow: Record<string, unknown> | undefined,
    newRow: Record<string, unknown> | undefined,
  ): Set<string> {
    if (!oldRow || !newRow) return new Set();
    const allKeys = new Set([...Object.keys(oldRow), ...Object.keys(newRow)]);
    return new Set(
      [...allKeys].filter(
        (key) => JSON.stringify(oldRow[key]) !== JSON.stringify(newRow[key]),
      ),
    );
  }

  function formatValue(val: unknown): string {
    if (val === null || val === undefined) return '—';
    if (typeof val === 'string') return val;
    if (typeof val === 'boolean') return val ? 'true' : 'false';
    if (typeof val === 'number') return String(val);
    return JSON.stringify(val, null, 2);
  }

  function isSimpleValue(val: unknown): boolean {
    return (
      val === null ||
      val === undefined ||
      typeof val === 'string' ||
      typeof val === 'boolean' ||
      typeof val === 'number'
    );
  }

  let diffKeys = $derived(changedKeys(event.old_row, event.row));

  let displayRow = $derived(event.row ?? event.old_row);
  let allKeys = $derived(displayRow ? Object.keys(displayRow).sort() : []);

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose();
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- Backdrop: a full-viewport button so it is dismissable by click, Enter/Space
     and (via the window handler) Escape. -->
<button
  type="button"
  class="fixed inset-0 z-40 bg-black/30 transition-opacity duration-300"
  aria-label="Close panel"
  onclick={onClose}
></button>

<!-- Panel -->
<div
  class="animate-in slide-in-from-right fixed top-0 right-0 z-50 flex h-full w-full max-w-xl flex-col border-l border-base-300 bg-base-100 shadow-2xl transition-transform duration-300"
>
  <!-- Header -->
  <div
    class="flex items-center justify-between border-b border-base-300 bg-base-200/40 px-4 py-3"
  >
    <div class="flex items-center gap-3">
      <Badge
        text={event.type}
        variant={actionVariant(event.type)}
        glyph={actionGlyph(event.type)}
      />
      <span class="font-mono text-sm text-base-content/60">
        {new Date(event.timestamp).toLocaleString()}
      </span>
    </div>
    <button
      class="btn btn-square btn-ghost btn-sm"
      onclick={onClose}
      aria-label="Close panel"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        class="h-4 w-4"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M6 18L18 6M6 6l12 12"
        />
      </svg>
    </button>
  </div>

  <!-- Content -->
  <div class="flex-1 overflow-y-auto p-4">
    <!-- Metadata -->
    <div
      class="mb-4 grid grid-cols-2 gap-2 rounded border border-base-300 bg-base-200/40 p-3"
    >
      <div>
        <div
          class="font-mono text-2xs tracking-wider text-base-content/60 uppercase"
        >
          Database
        </div>
        <div class="font-mono text-sm">{event.database}</div>
      </div>
      <div>
        <div
          class="font-mono text-2xs tracking-wider text-base-content/60 uppercase"
        >
          Table
        </div>
        <div class="font-mono text-sm">{event.table}</div>
      </div>
      <div class="col-span-2">
        <div
          class="font-mono text-2xs tracking-wider text-base-content/60 uppercase"
        >
          UUID
        </div>
        <div class="font-mono text-sm select-all">{event.uuid}</div>
      </div>
    </div>

    <!-- Visualized Data -->
    {#if event.type === 'update' && event.old_row && event.row}
      <!-- Update: show diff -->
      <h3
        class="mb-2 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
      >
        Changed Fields
      </h3>
      {#if diffKeys.size > 0}
        <div class="mb-4 space-y-2">
          {#each allKeys.filter((k) => diffKeys.has(k)) as key (key)}
            <div class="rounded border border-base-300 p-2">
              <div
                class="mb-1 font-mono text-xs font-semibold text-base-content/60"
              >
                {key}
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div class="rounded bg-error/10 p-2">
                  <div
                    class="mb-0.5 font-mono text-2xs font-semibold tracking-wider text-error uppercase"
                  >
                    Old
                  </div>
                  <pre
                    class="font-mono text-xs break-all whitespace-pre-wrap">{formatValue(
                      event.old_row![key],
                    )}</pre>
                </div>
                <div class="rounded bg-success/10 p-2">
                  <div
                    class="mb-0.5 font-mono text-2xs font-semibold tracking-wider text-success uppercase"
                  >
                    New
                  </div>
                  <pre
                    class="font-mono text-xs break-all whitespace-pre-wrap">{formatValue(
                      event.row![key],
                    )}</pre>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="mb-4 font-mono text-sm text-base-content/40">
          <span class="text-base-content/30">//</span> no field-level changes detected
        </div>
      {/if}

      {#if allKeys.filter((k) => !diffKeys.has(k)).length > 0}
        <h3
          class="mb-2 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
        >
          Unchanged Fields
        </h3>
        <div class="mb-4 overflow-x-auto rounded border border-base-300">
          <table class="table table-xs font-mono">
            <tbody>
              {#each allKeys.filter((k) => !diffKeys.has(k)) as key (key)}
                <tr class="border-base-300/60">
                  <td class="w-1/3 text-xs font-semibold text-base-content/60"
                    >{key}</td
                  >
                  <td>
                    {#if isSimpleValue(event.row![key])}
                      <span class="text-xs">{formatValue(event.row![key])}</span
                      >
                    {:else}
                      <pre
                        class="text-xs break-all whitespace-pre-wrap">{formatValue(
                          event.row![key],
                        )}</pre>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {:else if event.type === 'insert' && event.row}
      <!-- Insert: show new data -->
      <h3
        class="mb-2 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
      >
        New Row
      </h3>
      <div
        class="mb-4 overflow-x-auto rounded border border-success/30 bg-success/5"
      >
        <table class="table table-xs font-mono">
          <tbody>
            {#each allKeys as key (key)}
              <tr class="border-base-300/60">
                <td class="w-1/3 text-xs font-semibold text-base-content/60"
                  >{key}</td
                >
                <td>
                  {#if isSimpleValue(event.row[key])}
                    <span class="text-xs">{formatValue(event.row[key])}</span>
                  {:else}
                    <pre
                      class="text-xs break-all whitespace-pre-wrap">{formatValue(
                        event.row[key],
                      )}</pre>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else if event.type === 'delete' && event.old_row}
      <!-- Delete: show removed data -->
      <h3
        class="mb-2 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
      >
        Deleted Row
      </h3>
      <div
        class="mb-4 overflow-x-auto rounded border border-error/30 bg-error/5"
      >
        <table class="table table-xs font-mono">
          <tbody>
            {#each allKeys as key (key)}
              <tr class="border-base-300/60">
                <td class="w-1/3 text-xs font-semibold text-base-content/60"
                  >{key}</td
                >
                <td>
                  {#if isSimpleValue(event.old_row[key])}
                    <span class="text-xs"
                      >{formatValue(event.old_row[key])}</span
                    >
                  {:else}
                    <pre
                      class="text-xs break-all whitespace-pre-wrap">{formatValue(
                        event.old_row[key],
                      )}</pre>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else if displayRow}
      <!-- Fallback: generic key-value -->
      <h3
        class="mb-2 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
      >
        Row Data
      </h3>
      <div class="mb-4 overflow-x-auto rounded border border-base-300">
        <table class="table table-xs font-mono">
          <tbody>
            {#each allKeys as key (key)}
              <tr class="border-base-300/60">
                <td class="w-1/3 text-xs font-semibold text-base-content/60"
                  >{key}</td
                >
                <td>
                  {#if isSimpleValue(displayRow![key])}
                    <span class="text-xs">{formatValue(displayRow![key])}</span>
                  {:else}
                    <pre
                      class="text-xs break-all whitespace-pre-wrap">{formatValue(
                        displayRow![key],
                      )}</pre>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else}
      <div class="py-4 text-center font-mono text-sm text-base-content/40">
        <span class="text-base-content/30">//</span> no row data available
      </div>
    {/if}

    <!-- Raw JSON toggle -->
    <div class="border-t border-base-300 pt-3">
      <button
        class="btn border-base-300 btn-ghost btn-xs"
        onclick={() => (showRawJson = !showRawJson)}
      >
        {showRawJson ? 'Hide' : 'Show'} Raw JSON
      </button>
      {#if showRawJson}
        <div class="mt-2 space-y-2">
          {#if event.old_row}
            <div>
              <div
                class="mb-1 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
              >
                old_row
              </div>
              <pre
                class="max-h-64 overflow-auto rounded bg-base-200 p-3 text-xs">{JSON.stringify(
                  event.old_row,
                  null,
                  2,
                )}</pre>
            </div>
          {/if}
          {#if event.row}
            <div>
              <div
                class="mb-1 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
              >
                row
              </div>
              <pre
                class="max-h-64 overflow-auto rounded bg-base-200 p-3 text-xs">{JSON.stringify(
                  event.row,
                  null,
                  2,
                )}</pre>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  @keyframes slide-in-from-right {
    from {
      transform: translateX(100%);
    }
    to {
      transform: translateX(0);
    }
  }

  .animate-in {
    animation: slide-in-from-right 0.3s ease-out;
  }
</style>
