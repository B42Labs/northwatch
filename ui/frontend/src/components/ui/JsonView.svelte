<script lang="ts">
  let {
    data,
    label = 'json',
    open = $bindable(false),
  }: { data: unknown; label?: string; open?: boolean } = $props();

  let highlighted = $derived(highlightJson(JSON.stringify(data, null, 2)));

  function highlightJson(json: string): string {
    return json.replace(
      /("(?:\\.|[^"\\])*")\s*(:)?|(\b(?:true|false|null)\b)|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
      (match, str, colon, bool, num) => {
        if (str) {
          const escaped = str.replace(/&/g, '&amp;').replace(/</g, '&lt;');
          return colon
            ? `<span class="json-key">${escaped}</span>:`
            : `<span class="json-string">${escaped}</span>`;
        }
        if (bool) return `<span class="json-bool">${bool}</span>`;
        if (num) return `<span class="json-num">${num}</span>`;
        return match;
      },
    );
  }
</script>

<div class="overflow-hidden rounded border border-base-300 bg-base-100">
  <button
    type="button"
    class="flex w-full items-center gap-2 border-b border-base-300 bg-base-200/40 px-3 py-2 text-left font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase transition-colors hover:text-primary"
    onclick={() => (open = !open)}
    aria-expanded={open}
  >
    <span class="text-primary select-none" aria-hidden="true"
      >{open ? '▾' : '▸'}</span
    >
    {label}
  </button>
  {#if open}
    <!-- eslint-disable svelte/no-at-html-tags -- highlighted is derived from JSON.stringify, not user input -->
    <pre
      class="json-block overflow-x-auto p-3 text-xs leading-relaxed whitespace-pre-wrap">{@html highlighted}</pre>
    <!-- eslint-enable svelte/no-at-html-tags -->
  {/if}
</div>

<style>
  /* daisyUI theme variables — theme-aware in plain CSS. */
  .json-block :global(.json-key) {
    color: var(--color-accent);
  }
  .json-block :global(.json-string) {
    color: var(--color-success);
  }
  .json-block :global(.json-bool) {
    color: var(--color-secondary);
    font-weight: 600;
  }
  .json-block :global(.json-num) {
    color: var(--color-secondary);
  }
</style>
