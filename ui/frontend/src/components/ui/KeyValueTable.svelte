<script lang="ts">
  let {
    data,
    compact = false,
  }: { data: Record<string, string>; compact?: boolean } = $props();

  let entries = $derived(Object.entries(data));
</script>

{#if entries.length === 0}
  <span class="font-mono text-xs italic text-base-content/45">empty</span>
{:else}
  <table
    class="table table-xs font-mono {compact
      ? ''
      : 'rounded border border-base-300'}"
  >
    <tbody>
      {#each entries as [key, value] (key)}
        <tr class="border-base-300/60">
          <td
            class="whitespace-nowrap align-top text-xs font-medium text-base-content/55"
            >{key}</td
          >
          <td class="break-all text-xs">{value}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}
