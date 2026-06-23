<script lang="ts">
  import { listTable } from '../../lib/api';
  import SegmentedControl from '../ui/SegmentedControl.svelte';
  import FilterInput from '../ui/FilterInput.svelte';

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
  let mode = $state<'select' | 'manual'>('select');
  let manualMode = $derived(mode === 'manual');

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

<div class="flex flex-col gap-1.5">
  <SegmentedControl
    options={[
      { value: 'select', label: 'Select' },
      { value: 'manual', label: 'Manual UUID' },
    ]}
    bind:value={mode}
    size="xs"
  />

  {#if manualMode}
    <input
      type="text"
      class="input input-sm input-bordered w-full font-mono"
      placeholder="Enter UUID..."
      {value}
      oninput={(e) => onSelect(e.currentTarget.value)}
    />
  {:else}
    <FilterInput bind:value={filter} placeholder="Filter entities..." width="w-full" />
    {#if loading}
      <span class="font-mono text-xs text-base-content/50">Loading...</span>
    {:else}
      <select
        class="select select-bordered select-sm w-full bg-base-200/60 font-mono"
        {value}
        onchange={(e) => onSelect(e.currentTarget.value)}
      >
        <option value="">-- select entity --</option>
        {#each filtered as entity (String(entity._uuid || ''))}
          {@const uuid = String(entity._uuid || '')}
          {@const name = String(entity.name || '')}
          <option value={uuid}>
            {name ? `${name} (${uuid.slice(0, 8)})` : uuid.slice(0, 36)}
          </option>
        {/each}
      </select>
      {#if entities.length > 200}
        <span class="font-mono text-xs text-base-content/50">
          Showing 200 of {entities.length} — use filter to narrow
        </span>
      {/if}
    {/if}
  {/if}
</div>
