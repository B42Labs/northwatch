<script lang="ts">
  let { data }: { data: Record<string, unknown> } = $props();

  let displayName = $derived((data.display_name as string) || '');
  let projectName = $derived((data.project_name as string) || '');
  let deviceOwner = $derived((data.device_owner as string) || '');
  let deviceName = $derived((data.device_name as string) || '');
</script>

{#if displayName || projectName || deviceName}
  <div class="flex flex-wrap items-center gap-1">
    {#if displayName}
      <span class="badge badge-info badge-sm">{displayName}</span>
    {/if}
    {#if projectName}
      <span class="badge badge-ghost badge-sm">{projectName}</span>
    {/if}
    {#if deviceOwner}
      <span class="badge badge-ghost badge-sm">{deviceOwner}</span>
    {/if}
    {#if deviceName}
      <span class="badge badge-accent badge-sm">{deviceName}</span>
    {/if}
  </div>
{/if}
