<script lang="ts">
  let { data, label = 'JSON' }: { data: unknown; label?: string } = $props();
  let open = $state(false);

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

<div class="collapse collapse-arrow rounded-lg bg-base-200">
  <input type="checkbox" bind:checked={open} />
  <div class="collapse-title text-sm font-medium">{label}</div>
  <div class="collapse-content">
    <!-- eslint-disable svelte/no-at-html-tags -- highlighted is derived from JSON.stringify, not user input -->
    <pre
      class="json-block overflow-x-auto whitespace-pre-wrap text-xs">{@html highlighted}</pre>
    <!-- eslint-enable svelte/no-at-html-tags -->
  </div>
</div>

<style>
  .json-block :global(.json-key) {
    color: oklch(0.55 0.15 250);
  }
  .json-block :global(.json-string) {
    color: oklch(0.55 0.15 150);
  }
  .json-block :global(.json-bool),
  .json-block :global(.json-num) {
    color: oklch(0.55 0.15 30);
  }
</style>
