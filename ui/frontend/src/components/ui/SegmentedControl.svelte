<script lang="ts">
  // Replaces the hand-rolled `join` + `join-item btn` button groups used for
  // time ranges, severity filters, and mode toggles (~7 places).
  interface Option {
    value: string;
    label: string;
  }

  let {
    options,
    value = $bindable(),
    onchange,
    size = 'sm',
  }: {
    options: Option[];
    value: string;
    onchange?: (value: string) => void;
    size?: 'xs' | 'sm';
  } = $props();

  function select(v: string) {
    value = v;
    onchange?.(v);
  }
</script>

<div class="join">
  {#each options as o (o.value)}
    <button
      type="button"
      class="btn join-item btn-{size} font-mono normal-case {value === o.value
        ? 'btn-primary'
        : 'btn-ghost border-base-300'}"
      aria-pressed={value === o.value}
      onclick={() => select(o.value)}>{o.label}</button
    >
  {/each}
</div>
