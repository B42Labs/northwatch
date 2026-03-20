<script lang="ts">
  let {
    data,
    compact = false,
  }: { data: Record<string, string>; compact?: boolean } = $props();

  let entries = $derived(Object.entries(data));
</script>

{#if entries.length === 0}
  <span class="text-sm italic text-base-content/50">empty</span>
{:else}
  <table class="table table-xs {compact ? '' : 'table-zebra'}">
    <tbody>
      {#each entries as [key, value]}
        <tr>
          <td class="whitespace-nowrap font-mono text-xs font-semibold"
            >{key}</td
          >
          <td class="break-all font-mono text-xs">{value}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}
