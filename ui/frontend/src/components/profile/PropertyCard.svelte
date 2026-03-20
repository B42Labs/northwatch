<script lang="ts">
  import CellRenderer from '../table/CellRenderer.svelte';

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
  <div class="card bg-base-100 shadow-sm">
    <div class="card-body p-4">
      <h2 class="card-title text-sm">{title}</h2>
      <table class="table table-sm">
        <tbody>
          {#each fields as [key, value]}
            <tr>
              <td class="w-48 whitespace-nowrap text-xs font-semibold">{key}</td
              >
              <td><CellRenderer {value} column={key} /></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{/if}
