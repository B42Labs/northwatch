<script lang="ts">
  import type { DiffResult, TableDiff } from '../../lib/api';

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

<div class="rounded-lg border border-base-300 bg-base-100 p-4">
  <div class="mb-3 flex items-center justify-between">
    <h3 class="text-lg font-semibold">
      Diff: Snapshot #{diff.from_id} → #{diff.to_id}
    </h3>
    <span class="badge badge-ghost">{countChanges(diff.tables)} changes</span>
  </div>

  {#if diff.tables.length === 0}
    <div class="py-6 text-center text-sm text-base-content/40">
      No differences found between these snapshots.
    </div>
  {:else}
    <div class="mb-3">
      <input
        type="text"
        class="input input-sm input-bordered w-full"
        placeholder="Search diff by UUID, field name, value, table..."
        bind:value={searchQuery}
      />
      {#if searchQuery.trim()}
        <div class="mt-1 text-xs text-base-content/50">
          {countChanges(filteredTables)} of {countChanges(diff.tables)} changes match
        </div>
      {/if}
    </div>

    {#if filteredTables.length === 0}
      <div class="py-6 text-center text-sm text-base-content/40">
        No changes matching "{searchQuery.trim()}"
      </div>
    {:else}
      <div class="flex flex-col gap-4">
        {#each filteredTables as tableDiff (tableDiff.database + '.' + tableDiff.table)}
          <div class="rounded border border-base-300 p-3">
            <div class="mb-2 flex items-center gap-2 font-medium">
              <span class="badge badge-ghost badge-sm"
                >{tableDiff.database}</span
              >
              <span>{tableDiff.table}</span>
              <span class="ml-auto flex gap-2 text-xs">
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
                <div class="mb-1 text-xs font-semibold text-success">Added</div>
                {#each tableDiff.added as row, i (i)}
                  {#if tableDiff.added.length > 1}
                    <div class="mb-0.5 font-mono text-xs text-base-content/40">
                      {i + 1}.
                    </div>
                  {/if}
                  <pre
                    class="mb-1 overflow-auto rounded border-l-2 border-success bg-success/5 p-2 text-xs">{JSON.stringify(
                      row,
                      null,
                      2,
                    )}</pre>
                {/each}
              </div>
            {/if}

            {#if tableDiff.removed?.length}
              <div class="mb-2">
                <div class="mb-1 text-xs font-semibold text-error">Removed</div>
                {#each tableDiff.removed as row, i (i)}
                  {#if tableDiff.removed.length > 1}
                    <div class="mb-0.5 font-mono text-xs text-base-content/40">
                      {i + 1}.
                    </div>
                  {/if}
                  <pre
                    class="mb-1 overflow-auto rounded border-l-2 border-error bg-error/5 p-2 text-xs">{JSON.stringify(
                      row,
                      null,
                      2,
                    )}</pre>
                {/each}
              </div>
            {/if}

            {#if tableDiff.modified?.length}
              <div>
                <div class="mb-1 text-xs font-semibold text-warning">
                  Modified
                </div>
                {#each tableDiff.modified as mod, i (mod.uuid)}
                  {#if tableDiff.modified.length > 1}
                    <div class="mb-0.5 font-mono text-xs text-base-content/40">
                      {i + 1}.
                    </div>
                  {/if}
                  <div
                    class="mb-1 cursor-pointer rounded border-l-2 border-warning bg-warning/5 p-2"
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
                    <div class="flex items-center gap-2 text-xs">
                      <span class="font-mono text-base-content/60"
                        >{mod.uuid.slice(0, 12)}</span
                      >
                      <span class="text-base-content/40"
                        >{mod.fields.length} field{mod.fields.length !== 1
                          ? 's'
                          : ''} changed</span
                      >
                    </div>
                    {#if expandedModified === mod.uuid}
                      <table class="table table-xs mt-2">
                        <thead>
                          <tr>
                            <th>Field</th>
                            <th>Old</th>
                            <th>New</th>
                          </tr>
                        </thead>
                        <tbody>
                          {#each mod.fields as field (field.field)}
                            <tr>
                              <td class="font-mono font-semibold"
                                >{field.field}</td
                              >
                              <td
                                class="max-w-xs truncate font-mono text-error/70"
                              >
                                {JSON.stringify(field.old_value)}
                              </td>
                              <td
                                class="max-w-xs truncate font-mono text-success/70"
                              >
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
