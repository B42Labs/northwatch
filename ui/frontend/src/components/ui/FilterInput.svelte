<script lang="ts">
  // Standardizes the `input input-sm input-bordered` filter box used in
  // ~13 routes, with a consistent search glyph.
  let {
    value = $bindable(''),
    placeholder = 'filter…',
    size = 'sm',
    width = 'w-full max-w-xs',
    oninput,
  }: {
    value?: string;
    placeholder?: string;
    size?: 'xs' | 'sm';
    width?: string;
    oninput?: (value: string) => void;
  } = $props();
</script>

<label class="relative block {width}">
  <span
    class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 select-none font-mono text-sm text-base-content/40"
    aria-hidden="true">⌕</span
  >
  <input
    type="text"
    {placeholder}
    class="input input-{size} input-bordered w-full pl-7 font-mono"
    bind:value
    oninput={() => oninput?.(value)}
  />
</label>
