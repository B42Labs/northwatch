<script lang="ts">
  import Badge from '../ui/Badge.svelte';

  let { data }: { data: Record<string, unknown> } = $props();

  let displayName = $derived((data.display_name as string) || '');
  let projectName = $derived((data.project_name as string) || '');
  let deviceOwner = $derived((data.device_owner as string) || '');
  let deviceName = $derived((data.device_name as string) || '');
</script>

{#if displayName || projectName || deviceName || deviceOwner}
  <div class="flex flex-wrap items-center gap-1">
    {#if displayName}<Badge text={displayName} variant="info" />{/if}
    {#if projectName}<Badge text={projectName} variant="ghost" />{/if}
    {#if deviceOwner}<Badge text={deviceOwner} variant="ghost" />{/if}
    {#if deviceName}<Badge text={deviceName} variant="accent" />{/if}
  </div>
{/if}
