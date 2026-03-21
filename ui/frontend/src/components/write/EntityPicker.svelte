<script lang="ts">
  import { listTable } from '../../lib/api';

  let {
    tableSlug,
    value = '',
    onSelect,
  }: {
    tableSlug: string;
    value?: string;
    onSelect: (uuid: string) => void;
  } = $props();

  let entities: Record<string, unknown>[] = $state([]);
  let filter = $state('');
  let loading = $state(false);
  let manualMode = $state(false);

  $effect(() => {
    if (tableSlug) {
      loadEntities(tableSlug);
    } else {
      entities = [];
    }
  });

  async function loadEntities(slug: string) {
    loading = true;
    try {
      entities = await listTable('nb', slug);
    } catch {
      entities = [];
    } finally {
      loading = false;
    }
  }

  let filtered = $derived.by(() => {
    if (!filter) return entities.slice(0, 200);
    const q = filter.toLowerCase();
    return entities
      .filter((e) => {
        const name = String(e.name || '').toLowerCase();
        const uuid = String(e._uuid || '').toLowerCase();
        return name.includes(q) || uuid.includes(q);
      })
      .slice(0, 200);
  });
</script>

<div class="flex flex-col gap-1">
  <div class="flex items-center gap-2">
    <label class="label text-xs">
      <input
        type="checkbox"
        class="checkbox checkbox-xs"
        bind:checked={manualMode}
      />
      Manual UUID
    </label>
  </div>

  {#if manualMode}
    <input
      type="text"
      class="input input-sm input-bordered w-full font-mono"
      placeholder="Enter UUID..."
      {value}
      oninput={(e) => onSelect(e.currentTarget.value)}
    />
  {:else}
    <input
      type="text"
      class="input input-sm input-bordered w-full"
      placeholder="Filter entities..."
      bind:value={filter}
    />
    {#if loading}
      <span class="text-xs text-base-content/50">Loading...</span>
    {:else}
      <select
        class="select select-bordered select-sm w-full font-mono"
        {value}
        onchange={(e) => onSelect(e.currentTarget.value)}
      >
        <option value="">-- select entity --</option>
        {#each filtered as entity}
          {@const uuid = String(entity._uuid || '')}
          {@const name = String(entity.name || '')}
          <option value={uuid}>
            {name ? `${name} (${uuid.slice(0, 8)})` : uuid.slice(0, 36)}
          </option>
        {/each}
      </select>
      {#if entities.length > 200}
        <span class="text-xs text-base-content/50">
          Showing 200 of {entities.length} — use filter to narrow
        </span>
      {/if}
    {/if}
  {/if}
</div>
