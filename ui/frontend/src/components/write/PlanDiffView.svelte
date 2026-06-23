<script lang="ts">
  import type { PlanDiff } from '../../lib/writeApi';
  import Badge from '../ui/Badge.svelte';
  import {
    actionVariant,
    actionGlyph,
    accentBorderClass,
  } from '../../lib/status';

  let { diffs }: { diffs: PlanDiff[] } = $props();

  function formatValue(v: unknown): string {
    if (v === null || v === undefined) return '-';
    if (typeof v === 'string') return v;
    return JSON.stringify(v);
  }
</script>

{#if diffs.length === 0}
  <div class="py-4 text-center font-mono text-sm text-base-content/40">
    <span class="text-base-content/30">//</span> no changes detected
  </div>
{:else}
  <div class="flex flex-col gap-3">
    {#each diffs as diff, i (i)}
      <div
        class="rounded border border-l-2 border-base-300 bg-base-100 {accentBorderClass[
          actionVariant(diff.action)
        ]}"
      >
        <div class="flex flex-col gap-2 p-3">
          <div class="flex items-center gap-2">
            <Badge
              text={diff.action}
              variant={actionVariant(diff.action)}
              glyph={actionGlyph(diff.action)}
            />
            <span class="font-mono text-sm">{diff.table}</span>
            {#if diff.uuid}
              <span class="font-mono text-xs text-base-content/50">
                {diff.uuid.slice(0, 8)}
              </span>
            {/if}
          </div>

          {#if diff.fields && diff.fields.length > 0}
            <div class="overflow-x-auto rounded border border-base-300">
              <table class="table table-xs font-mono">
                <thead>
                  <tr>
                    <th
                      class="w-40 bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >Field</th
                    >
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >Before</th
                    >
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >After</th
                    >
                  </tr>
                </thead>
                <tbody>
                  {#each diff.fields as change (change.field)}
                    <tr class="border-base-300/60">
                      <td class="font-mono text-xs font-semibold">
                        {change.field}
                      </td>
                      <td
                        class="max-w-xs truncate font-mono text-xs text-base-content/60"
                      >
                        {formatValue(change.old_value)}
                      </td>
                      <td class="max-w-xs truncate font-mono text-xs">
                        {formatValue(change.new_value)}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}
