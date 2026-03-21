<script lang="ts">
  import type { PlanDiff } from '../../lib/writeApi';
  import Badge from '../ui/Badge.svelte';

  let { diffs }: { diffs: PlanDiff[] } = $props();

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

  function borderClass(action: string): string {
    switch (action) {
      case 'create':
        return 'border-success';
      case 'update':
        return 'border-warning';
      case 'delete':
        return 'border-error';
      default:
        return 'border-base-300';
    }
  }

  function formatValue(v: unknown): string {
    if (v === null || v === undefined) return '-';
    if (typeof v === 'string') return v;
    return JSON.stringify(v);
  }
</script>

{#if diffs.length === 0}
  <div class="py-4 text-center text-sm text-base-content/50">
    No changes detected.
  </div>
{:else}
  <div class="flex flex-col gap-3">
    {#each diffs as diff}
      <div
        class="card border-l-4 bg-base-100 shadow-sm {borderClass(diff.action)}"
      >
        <div class="card-body p-3">
          <div class="flex items-center gap-2">
            <Badge text={diff.action} variant={actionVariant(diff.action)} />
            <span class="font-mono text-sm">{diff.table}</span>
            {#if diff.uuid}
              <span class="font-mono text-xs text-base-content/50">
                {diff.uuid.slice(0, 8)}
              </span>
            {/if}
          </div>

          {#if diff.fields && diff.fields.length > 0}
            <div class="overflow-x-auto">
              <table class="table table-xs mt-1">
                <thead>
                  <tr>
                    <th class="w-40">Field</th>
                    <th>Before</th>
                    <th>After</th>
                  </tr>
                </thead>
                <tbody>
                  {#each diff.fields as change}
                    <tr>
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
