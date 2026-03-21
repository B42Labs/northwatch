<script lang="ts">
  import type { EventRecord } from '../../lib/api';

  interface Props {
    event: EventRecord;
    onClose: () => void;
  }

  let { event, onClose }: Props = $props();

  let showRawJson = $state(false);

  function badgeClass(type: string): string {
    switch (type) {
      case 'insert':
        return 'badge-success';
      case 'delete':
        return 'badge-error';
      case 'update':
        return 'badge-warning';
      default:
        return 'badge-ghost';
    }
  }

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
</script>

<!-- Backdrop -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="fixed inset-0 z-40 bg-black/30 transition-opacity duration-300"
  onclick={onClose}
></div>

<!-- Panel -->
<div
  class="animate-in slide-in-from-right fixed right-0 top-0 z-50 flex h-full w-full max-w-xl flex-col border-l border-base-300 bg-base-100 shadow-2xl transition-transform duration-300"
>
  <!-- Header -->
  <div
    class="flex items-center justify-between border-b border-base-300 px-4 py-3"
  >
    <div class="flex items-center gap-3">
      <span
        class="badge {badgeClass(event.type)} badge-sm font-semibold uppercase"
      >
        {event.type}
      </span>
      <span class="text-sm text-base-content/60">
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
    <div class="mb-4 grid grid-cols-2 gap-2 rounded-lg bg-base-200 p-3">
      <div>
        <div class="text-xs font-semibold text-base-content/50">Database</div>
        <div class="font-mono text-sm">{event.database}</div>
      </div>
      <div>
        <div class="text-xs font-semibold text-base-content/50">Table</div>
        <div class="font-mono text-sm">{event.table}</div>
      </div>
      <div class="col-span-2">
        <div class="text-xs font-semibold text-base-content/50">UUID</div>
        <div class="select-all font-mono text-sm">{event.uuid}</div>
      </div>
    </div>

    <!-- Visualized Data -->
    {#if event.type === 'update' && event.old_row && event.row}
      <!-- Update: show diff -->
      <h3 class="mb-2 text-sm font-semibold">Changed Fields</h3>
      {#if diffKeys.size > 0}
        <div class="mb-4 space-y-2">
          {#each allKeys.filter((k) => diffKeys.has(k)) as key}
            <div class="rounded-lg border border-base-300 p-2">
              <div class="mb-1 text-xs font-semibold text-base-content/60">
                {key}
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div class="rounded bg-error/10 p-2">
                  <div class="mb-0.5 text-[10px] font-semibold text-error">
                    Old
                  </div>
                  <pre
                    class="whitespace-pre-wrap break-all text-xs">{formatValue(
                      event.old_row![key],
                    )}</pre>
                </div>
                <div class="rounded bg-success/10 p-2">
                  <div class="mb-0.5 text-[10px] font-semibold text-success">
                    New
                  </div>
                  <pre
                    class="whitespace-pre-wrap break-all text-xs">{formatValue(
                      event.row![key],
                    )}</pre>
                </div>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <div class="mb-4 text-xs text-base-content/40">
          No field-level changes detected.
        </div>
      {/if}

      {#if allKeys.filter((k) => !diffKeys.has(k)).length > 0}
        <h3 class="mb-2 text-sm font-semibold">Unchanged Fields</h3>
        <div class="mb-4 overflow-x-auto rounded-lg border border-base-300">
          <table class="table table-xs">
            <tbody>
              {#each allKeys.filter((k) => !diffKeys.has(k)) as key}
                <tr>
                  <td
                    class="w-1/3 font-mono text-xs font-semibold text-base-content/60"
                    >{key}</td
                  >
                  <td>
                    {#if isSimpleValue(event.row![key])}
                      <span class="text-xs">{formatValue(event.row![key])}</span
                      >
                    {:else}
                      <pre
                        class="whitespace-pre-wrap break-all text-xs">{formatValue(
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
      <h3 class="mb-2 text-sm font-semibold">New Row</h3>
      <div
        class="mb-4 overflow-x-auto rounded-lg border border-success/30 bg-success/5"
      >
        <table class="table table-xs">
          <tbody>
            {#each allKeys as key}
              <tr>
                <td
                  class="w-1/3 font-mono text-xs font-semibold text-base-content/60"
                  >{key}</td
                >
                <td>
                  {#if isSimpleValue(event.row[key])}
                    <span class="text-xs">{formatValue(event.row[key])}</span>
                  {:else}
                    <pre
                      class="whitespace-pre-wrap break-all text-xs">{formatValue(
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
      <h3 class="mb-2 text-sm font-semibold">Deleted Row</h3>
      <div
        class="mb-4 overflow-x-auto rounded-lg border border-error/30 bg-error/5"
      >
        <table class="table table-xs">
          <tbody>
            {#each allKeys as key}
              <tr>
                <td
                  class="w-1/3 font-mono text-xs font-semibold text-base-content/60"
                  >{key}</td
                >
                <td>
                  {#if isSimpleValue(event.old_row[key])}
                    <span class="text-xs"
                      >{formatValue(event.old_row[key])}</span
                    >
                  {:else}
                    <pre
                      class="whitespace-pre-wrap break-all text-xs">{formatValue(
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
      <h3 class="mb-2 text-sm font-semibold">Row Data</h3>
      <div class="mb-4 overflow-x-auto rounded-lg border border-base-300">
        <table class="table table-xs">
          <tbody>
            {#each allKeys as key}
              <tr>
                <td
                  class="w-1/3 font-mono text-xs font-semibold text-base-content/60"
                  >{key}</td
                >
                <td>
                  {#if isSimpleValue(displayRow![key])}
                    <span class="text-xs">{formatValue(displayRow![key])}</span>
                  {:else}
                    <pre
                      class="whitespace-pre-wrap break-all text-xs">{formatValue(
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
      <div class="py-4 text-center text-sm text-base-content/40">
        No row data available.
      </div>
    {/if}

    <!-- Raw JSON toggle -->
    <div class="border-t border-base-300 pt-3">
      <button
        class="btn btn-ghost btn-xs"
        onclick={() => (showRawJson = !showRawJson)}
      >
        {showRawJson ? 'Hide' : 'Show'} Raw JSON
      </button>
      {#if showRawJson}
        <div class="mt-2 space-y-2">
          {#if event.old_row}
            <div>
              <div class="mb-1 text-xs font-semibold text-base-content/50">
                old_row
              </div>
              <pre
                class="max-h-64 overflow-auto rounded-lg bg-base-200 p-3 text-xs">{JSON.stringify(
                  event.old_row,
                  null,
                  2,
                )}</pre>
            </div>
          {/if}
          {#if event.row}
            <div>
              <div class="mb-1 text-xs font-semibold text-base-content/50">
                row
              </div>
              <pre
                class="max-h-64 overflow-auto rounded-lg bg-base-200 p-3 text-xs">{JSON.stringify(
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
