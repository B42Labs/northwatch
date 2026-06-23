<script lang="ts">
  let { label = 'querying' }: { label?: string } = $props();
</script>

<div
  class="flex items-center justify-center gap-1.5 py-12 font-mono text-sm text-base-content/55"
  role="status"
  aria-live="polite"
>
  <span class="text-primary">&gt;</span>
  <span>{label}</span>
</div>
