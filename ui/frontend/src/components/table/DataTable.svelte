<script lang="ts">
  import CellRenderer from './CellRenderer.svelte';

  let {
    rows,
    columns,
    allColumns = [],
    onRowClick,
    refHref,
    changedUuids = new Map(),
  }: {
    rows: Record<string, unknown>[];
    columns: string[];
    allColumns?: string[];
    onRowClick?: (row: Record<string, unknown>) => void;
    refHref?: (column: string) => ((uuid: string) => string | null) | undefined;
    changedUuids?: Map<string, number>;
  } = $props();

  let filterInput = $state('');
  let filter = $state('');
  let sortColumn = $state('');
  let sortAsc = $state(true);
  let showAll = $state(false);
  let pageSize = $state(50);
  let currentPage = $state(1);

  // Debounce filter to avoid lag on large tables (e.g. logical flows with 100k+ rows).
  $effect(() => {
    const value = filterInput;
    const timer = setTimeout(() => {
      filter = value;
    }, 200);
    return () => clearTimeout(timer);
  });

  // Reset to page 1 when filter changes.
  $effect(() => {
    void filter;
    currentPage = 1;
  });

  let displayColumns = $derived(
    showAll && allColumns.length > 0 ? allColumns : columns,
  );

  let filteredRows = $derived.by(() => {
    let result = rows;
    if (filter) {
      const q = filter.toLowerCase();
      result = result.filter((row) =>
        displayColumns.some((col) => {
          const v = row[col];
          if (v === null || v === undefined) return false;
          return String(v).toLowerCase().includes(q);
        }),
      );
    }
    if (sortColumn) {
      result = [...result].sort((a, b) => {
        const va = a[sortColumn] ?? '';
        const vb = b[sortColumn] ?? '';
        const cmp = String(va).localeCompare(String(vb), undefined, {
          numeric: true,
        });
        return sortAsc ? cmp : -cmp;
      });
    }
    return result;
  });

  let totalPages = $derived(
    Math.max(1, Math.ceil(filteredRows.length / pageSize)),
  );
  let displayedRows = $derived(
    filteredRows.slice((currentPage - 1) * pageSize, currentPage * pageSize),
  );

  // Build visible page numbers with ellipsis.
  let pageNumbers = $derived.by(() => {
    const pages: (number | '...')[] = [];
    const total = totalPages;
    const cur = currentPage;
    if (total <= 7) {
      for (let i = 1; i <= total; i++) pages.push(i);
    } else {
      pages.push(1);
      if (cur > 3) pages.push('...');
      for (
        let i = Math.max(2, cur - 1);
        i <= Math.min(total - 1, cur + 1);
        i++
      ) {
        pages.push(i);
      }
      if (cur < total - 2) pages.push('...');
      pages.push(total);
    }
    return pages;
  });

  function toggleSort(col: string) {
    if (sortColumn === col) {
      sortAsc = !sortAsc;
    } else {
      sortColumn = col;
      sortAsc = true;
    }
  }
</script>

<div class="flex flex-col gap-3">
  <!-- Toolbar -->
  <div class="flex flex-wrap items-center gap-3">
    <input
      type="text"
      placeholder="Filter rows..."
      class="input input-sm input-bordered w-full max-w-xs"
      bind:value={filterInput}
    />
    <span class="text-sm text-base-content/60">
      {filteredRows.length} of {rows.length} rows
    </span>
    <select
      class="select select-bordered select-sm"
      bind:value={pageSize}
      onchange={() => (currentPage = 1)}
    >
      <option value={25}>25 / page</option>
      <option value={50}>50 / page</option>
      <option value={100}>100 / page</option>
      <option value={250}>250 / page</option>
    </select>
    {#if allColumns.length > columns.length}
      <label class="label cursor-pointer gap-2">
        <span class="label-text text-sm">All columns</span>
        <input
          type="checkbox"
          class="toggle toggle-sm"
          bind:checked={showAll}
        />
      </label>
    {/if}
  </div>

  <!-- Table -->
  <div
    class="max-h-[calc(100vh-16rem)] overflow-x-auto overflow-y-auto rounded-lg border border-base-300"
  >
    <table class="table table-pin-rows table-xs">
      <thead>
        <tr>
          {#each displayColumns as col (col)}
            <th
              class="cursor-pointer select-none whitespace-nowrap bg-base-100 hover:bg-base-200"
              onclick={() => toggleSort(col)}
            >
              <div class="flex items-center gap-1">
                {col}
                {#if sortColumn === col}
                  <span class="text-xs opacity-70">{sortAsc ? '▲' : '▼'}</span>
                {:else}
                  <span class="text-xs opacity-0 group-hover:opacity-30">▲</span
                  >
                {/if}
              </div>
            </th>
          {/each}
        </tr>
      </thead>
      <tbody>
        {#each displayedRows as row, i (row._uuid ?? i)}
          <tr
            class="{onRowClick
              ? 'hover cursor-pointer'
              : 'hover'}{changedUuids.has(String(row._uuid ?? ''))
              ? ' recently-changed'
              : ''}"
            onclick={() => onRowClick?.(row)}
          >
            {#each displayColumns as col (col)}
              <td
                class="max-w-xs whitespace-nowrap"
                style:background-color={i % 2 === 1
                  ? 'oklch(var(--b3) / 1)'
                  : null}
              >
                <CellRenderer value={row[col]} refHref={refHref?.(col)} />
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  <!-- Footer -->
  {#if filteredRows.length > 0}
    <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
      <span class="text-base-content/60">
        Showing {(currentPage - 1) * pageSize + 1}–{Math.min(
          currentPage * pageSize,
          filteredRows.length,
        )} of {filteredRows.length}
      </span>
      {#if totalPages > 1}
        <div class="join">
          <button
            class="btn join-item btn-sm"
            disabled={currentPage === 1}
            onclick={() => currentPage--}>«</button
          >
          {#each pageNumbers as p, i (i)}
            {#if p === '...'}
              <button class="btn btn-disabled join-item btn-sm">…</button>
            {:else}
              <button
                class="btn join-item btn-sm {p === currentPage
                  ? 'btn-active'
                  : ''}"
                onclick={() => (currentPage = p)}>{p}</button
              >
            {/if}
          {/each}
          <button
            class="btn join-item btn-sm"
            disabled={currentPage === totalPages}
            onclick={() => currentPage++}>»</button
          >
        </div>
      {/if}
    </div>
  {/if}

  {#if rows.length === 0}
    <div class="py-8 text-center text-base-content/50">No data</div>
  {/if}
</div>
