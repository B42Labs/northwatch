<script lang="ts">
  import type { DiffResult, TableDiff } from '../../lib/api';
  import Badge from '../ui/Badge.svelte';
  import FilterInput from '../ui/FilterInput.svelte';

  interface Props {
    diff: DiffResult;
  }

  let { diff }: Props = $props();

  let expandedModified: string | null = $state(null);
  let searchQuery = $state('');

  function countChanges(tables: TableDiff[]): number {
    return tables.reduce(
      (sum, t) =>
        sum +
        (t.added?.length ?? 0) +
        (t.removed?.length ?? 0) +
        (t.modified?.length ?? 0),
      0,
    );
  }

  function matchesQuery(value: unknown, q: string): boolean {
    const str = typeof value === 'string' ? value : JSON.stringify(value);
    return str.toLowerCase().includes(q);
  }

  let filteredTables = $derived.by(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return diff.tables;

    const result: TableDiff[] = [];
    for (const t of diff.tables) {
      const tableMatches =
        t.database.toLowerCase().includes(q) ||
        t.table.toLowerCase().includes(q);

      const added = t.added?.filter((row) => matchesQuery(row, q)) ?? [];
      const removed = t.removed?.filter((row) => matchesQuery(row, q)) ?? [];
      const modified =
        t.modified?.filter(
          (mod) =>
            mod.uuid.toLowerCase().includes(q) ||
            mod.fields.some(
              (f) =>
                f.field.toLowerCase().includes(q) ||
                matchesQuery(f.old_value, q) ||
                matchesQuery(f.new_value, q),
            ),
        ) ?? [];

      if (tableMatches || added.length || removed.length || modified.length) {
        result.push({
          ...t,
          added: tableMatches ? t.added : added,
          removed: tableMatches ? t.removed : removed,
          modified: tableMatches ? t.modified : modified,
        });
      }
    }
    return result;
  });
</script>

<div class="rounded border border-base-300 bg-base-100 p-4">
  <div class="mb-3 flex items-center justify-between">
    <h3
      class="font-mono text-xs font-semibold uppercase tracking-wider text-base-content/80"
    >
      Diff: Snapshot #{diff.from_id} → #{diff.to_id}
    </h3>
    <Badge text="{countChanges(diff.tables)} changes" variant="neutral" />
  </div>

  {#if diff.tables.length === 0}
    <div class="py-6 text-center font-mono text-sm text-base-content/40">
      <span class="text-base-content/30">//</span> no differences found between these
      snapshots
    </div>
  {:else}
    <div class="mb-3">
      <FilterInput
        bind:value={searchQuery}
        placeholder="search diff by UUID, field name, value, table…"
        width="w-full"
      />
      {#if searchQuery.trim()}
        <div class="mt-1 font-mono text-xs text-base-content/55">
          {countChanges(filteredTables)} of {countChanges(diff.tables)} changes match
        </div>
      {/if}
    </div>

    {#if filteredTables.length === 0}
      <div class="py-6 text-center font-mono text-sm text-base-content/40">
        <span class="text-base-content/30">//</span> no changes matching "{searchQuery.trim()}"
      </div>
    {:else}
      <div class="flex flex-col gap-4">
        {#each filteredTables as tableDiff (tableDiff.database + '.' + tableDiff.table)}
          <div class="rounded border border-base-300 p-3">
            <div class="mb-2 flex items-center gap-2 font-mono font-medium">
              <Badge text={tableDiff.database} variant="neutral" />
              <span>{tableDiff.table}</span>
              <span class="ml-auto flex gap-2 font-mono text-xs">
                {#if tableDiff.added?.length}
                  <span class="text-success"
                    >+{tableDiff.added.length} added</span
                  >
                {/if}
                {#if tableDiff.removed?.length}
                  <span class="text-error"
                    >-{tableDiff.removed.length} removed</span
                  >
                {/if}
                {#if tableDiff.modified?.length}
                  <span class="text-warning"
                    >~{tableDiff.modified.length} modified</span
                  >
                {/if}
              </span>
            </div>

            {#if tableDiff.added?.length}
              <div class="mb-2">
                <div
                  class="mb-1 font-mono text-xs font-semibold uppercase tracking-wider text-success"
                >
                  Added
                </div>
                {#each tableDiff.added as row, i (i)}
                  {#if tableDiff.added.length > 1}
                    <div class="mb-0.5 font-mono text-xs text-base-content/40">
                      {i + 1}.
                    </div>
                  {/if}
                  <pre
                    class="mb-1 overflow-auto rounded border-l-2 border-success bg-success/10 p-2 text-xs text-success">{JSON.stringify(
                      row,
                      null,
                      2,
                    )}</pre>
                {/each}
              </div>
            {/if}

            {#if tableDiff.removed?.length}
              <div class="mb-2">
                <div
                  class="mb-1 font-mono text-xs font-semibold uppercase tracking-wider text-error"
                >
                  Removed
                </div>
                {#each tableDiff.removed as row, i (i)}
                  {#if tableDiff.removed.length > 1}
                    <div class="mb-0.5 font-mono text-xs text-base-content/40">
                      {i + 1}.
                    </div>
                  {/if}
                  <pre
                    class="mb-1 overflow-auto rounded border-l-2 border-error bg-error/10 p-2 text-xs text-error">{JSON.stringify(
                      row,
                      null,
                      2,
                    )}</pre>
                {/each}
              </div>
            {/if}

            {#if tableDiff.modified?.length}
              <div>
                <div
                  class="mb-1 font-mono text-xs font-semibold uppercase tracking-wider text-warning"
                >
                  Modified
                </div>
                {#each tableDiff.modified as mod, i (mod.uuid)}
                  {#if tableDiff.modified.length > 1}
                    <div class="mb-0.5 font-mono text-xs text-base-content/40">
                      {i + 1}.
                    </div>
                  {/if}
                  <div
                    class="mb-1 cursor-pointer rounded border-l-2 border-warning bg-warning/10 p-2"
                    onclick={() =>
                      (expandedModified =
                        expandedModified === mod.uuid ? null : mod.uuid)}
                    role="button"
                    tabindex="0"
                    onkeydown={(e) =>
                      e.key === 'Enter' &&
                      (expandedModified =
                        expandedModified === mod.uuid ? null : mod.uuid)}
                  >
                    <div class="flex items-center gap-2 font-mono text-xs">
                      <span class="text-base-content/60"
                        >{mod.uuid.slice(0, 12)}</span
                      >
                      <span class="text-base-content/40"
                        >{mod.fields.length} field{mod.fields.length !== 1
                          ? 's'
                          : ''} changed</span
                      >
                    </div>
                    {#if expandedModified === mod.uuid}
                      <table class="table table-xs mt-2 font-mono">
                        <thead>
                          <tr>
                            <th
                              class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                              >Field</th
                            >
                            <th
                              class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                              >Old</th
                            >
                            <th
                              class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                              >New</th
                            >
                          </tr>
                        </thead>
                        <tbody>
                          {#each mod.fields as field (field.field)}
                            <tr class="border-base-300/60">
                              <td class="font-semibold">{field.field}</td>
                              <td class="max-w-xs truncate text-error/70">
                                {JSON.stringify(field.old_value)}
                              </td>
                              <td class="max-w-xs truncate text-success/70">
                                {JSON.stringify(field.new_value)}
                              </td>
                            </tr>
                          {/each}
                        </tbody>
                      </table>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
