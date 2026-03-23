<script lang="ts">
  import UuidLink from '../ui/UuidLink.svelte';
  import KeyValueTable from '../ui/KeyValueTable.svelte';

  let {
    value,
    refHref,
  }: {
    value: unknown;
    refHref?: (uuid: string) => string | null;
  } = $props();

  const UUID_RE =
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

  function isUuid(v: unknown): v is string {
    return typeof v === 'string' && UUID_RE.test(v);
  }

  function isStringMap(v: unknown): v is Record<string, string> {
    return (
      typeof v === 'object' &&
      v !== null &&
      !Array.isArray(v) &&
      Object.values(v as Record<string, unknown>).every(
        (val) => typeof val === 'string',
      )
    );
  }

  function getHref(uuid: string): string {
    if (refHref) {
      const h = refHref(uuid);
      if (h) return h;
    }
    return '';
  }
</script>

{#if value === null || value === undefined}
  <span class="italic text-base-content/40">-</span>
{:else if typeof value === 'boolean'}
  {#if value}
    <span class="badge badge-success badge-sm">true</span>
  {:else}
    <span class="badge badge-error badge-sm">false</span>
  {/if}
{:else if isUuid(value)}
  <UuidLink uuid={value} short={true} href={getHref(value)} />
{:else if Array.isArray(value)}
  {#if value.length === 0}
    <span class="italic text-base-content/40">empty</span>
  {:else if value.length <= 3 && value.every(isUuid)}
    <div class="flex flex-col gap-0.5">
      {#each value as item (item)}
        <UuidLink uuid={item} short={true} href={getHref(item)} />
      {/each}
    </div>
  {:else}
    <details class="text-xs">
      <summary class="cursor-pointer">{value.length} items</summary>
      <div class="mt-1 flex max-h-32 flex-col gap-0.5 overflow-y-auto">
        {#each value as item, i (i)}
          {#if isUuid(item)}
            <UuidLink uuid={item} short={true} href={getHref(item)} />
          {:else}
            <span class="font-mono">{String(item)}</span>
          {/if}
        {/each}
      </div>
    </details>
  {/if}
{:else if isStringMap(value)}
  {#if Object.keys(value).length === 0}
    <span class="italic text-base-content/40">empty</span>
  {:else}
    <details class="text-xs">
      <summary class="cursor-pointer"
        >{Object.keys(value).length} entries</summary
      >
      <div class="mt-1">
        <KeyValueTable data={value} compact={true} />
      </div>
    </details>
  {/if}
{:else if typeof value === 'string' && value.length > 80}
  <span class="font-mono text-xs" title={value}>{value.slice(0, 80)}...</span>
{:else}
  <span class="font-mono text-xs">{String(value)}</span>
{/if}
