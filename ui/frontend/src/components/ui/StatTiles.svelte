<script lang="ts">
  import { textClass, type Variant } from '../../lib/status';

  export interface Tile {
    label: string;
    value: string | number;
    variant?: Variant;
    hint?: string;
  }

  // Replaces the `stats stats-horizontal ml-auto border ...` block repeated
  // across 9 dashboards.
  let {
    tiles,
    class: extra = '',
  }: {
    tiles: Tile[];
    class?: string;
  } = $props();
</script>

<div
  class="flex flex-wrap gap-px overflow-hidden rounded border border-base-300 bg-base-300 {extra}"
>
  {#each tiles as t (t.label)}
    <div class="flex min-w-[6.5rem] flex-1 flex-col gap-0.5 bg-base-100 px-3 py-2">
      <span
        class="font-mono text-xl font-semibold tabular-nums leading-none {textClass[
          t.variant ?? 'neutral'
        ]}">{t.value}</span
      >
      <span
        class="font-mono text-2xs uppercase tracking-wider text-base-content/50"
        title={t.hint}>{t.label}</span
      >
    </div>
  {/each}
</div>
