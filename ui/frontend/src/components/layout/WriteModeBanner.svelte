<script lang="ts">
  import { writeEnabled } from '../../lib/capabilitiesStore';
</script>

{#if $writeEnabled}
  <div
    role="alert"
    class="flex items-center justify-center gap-2 border-b border-warning/30 bg-warning/15 px-4 py-1.5 font-mono text-2xs uppercase tracking-wider text-warning"
  >
    <span class="font-bold" aria-hidden="true">!</span>
    <span>write mode active — changes to OVN are enabled</span>
  </div>
{/if}
