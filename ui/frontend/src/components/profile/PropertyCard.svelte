<script lang="ts">
  import CellRenderer from '../table/CellRenderer.svelte';
  import Card from '../ui/Card.svelte';

  let {
    title,
    data,
    exclude = [],
  }: {
    title: string;
    data: Record<string, unknown>;
    exclude?: string[];
  } = $props();

  let fields = $derived(
    Object.entries(data)
      .filter(([k]) => !exclude.includes(k))
      .sort(([a], [b]) => a.localeCompare(b)),
  );
</script>

{#if fields.length > 0}
  <Card {title} padding="none">
    <table class="table table-sm font-mono">
      <tbody>
        {#each fields as [key, value] (key)}
          <tr class="border-base-300/60">
            <td
              class="w-48 whitespace-nowrap align-top text-xs font-medium text-base-content/55"
              >{key}</td
            >
            <td class="align-top"><CellRenderer {value} /></td>
          </tr>
        {/each}
      </tbody>
    </table>
  </Card>
{/if}
