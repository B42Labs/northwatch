<script lang="ts">
  let { message }: { message: string } = $props();
</script>

<div
  role="alert"
  class="flex items-start gap-2 rounded border border-error/40 bg-error/10 px-3 py-2.5 font-mono text-sm text-error"
>
  <span class="select-none font-bold" aria-hidden="true">!</span>
  <span class="min-w-0 break-words"
    ><span class="text-error/60">error:</span> {message}</span
  >
</div>
