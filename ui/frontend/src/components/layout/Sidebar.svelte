<script lang="ts">
  import { location, link } from '../../lib/router';
  import { databases } from '../../lib/tables';
  import { navSections, isActiveLink, type NavLink } from '../../lib/nav';
  import {
    capabilities,
    writeEnabled,
    snapshotMode,
  } from '../../lib/capabilitiesStore';

  // Database groups carry a lot of tables, so they start collapsed; the
  // curated sections above stay open.
  let collapsed: Record<string, boolean> = $state({ nb: true, sb: true });

  function toggle(key: string) {
    collapsed[key] = !collapsed[key];
  }

  // In snapshot mode, hide sections and links that depend on live-change
  // tracking (telemetry, propagation, flow diff) — the snapshot serves a static
  // point in time and those endpoints would 404.
  let sections = $derived(
    navSections
      .filter((s) => !s.requiresWrite || $writeEnabled)
      .filter((s) => !s.liveOnly || !$snapshotMode)
      .map((s) => ({
        ...s,
        links: s.links
          .filter((l) => !l.liveOnly || !$snapshotMode)
          .filter(
            (l) =>
              !l.requiresCapability ||
              $capabilities.includes(l.requiresCapability),
          ),
      })),
  );

  // The database tables expressed as nav links.
  let dbGroups = $derived(
    databases.map((db) => ({
      key: db.key,
      label: db.label,
      links: db.tables.map(
        (t): NavLink => ({ label: t.label, href: `/${db.key}/${t.slug}` }),
      ),
    })),
  );
</script>

{#snippet group(key: string, label: string, links: NavLink[])}
  {@const isOpen = !collapsed[key]}
  <div class="mb-0.5">
    <button
      type="button"
      class="group flex w-full items-center gap-1.5 rounded px-2 py-1.5 font-mono text-2xs font-semibold tracking-widest text-base-content/45 uppercase transition-colors hover:text-base-content/80"
      onclick={() => toggle(key)}
      aria-expanded={isOpen}
    >
      <span
        class="text-base-content/30 transition-colors select-none group-hover:text-primary"
        >{isOpen ? '▾' : '▸'}</span
      >
      <span class="truncate">{label}</span>
    </button>
    {#if isOpen}
      <ul class="ml-2 flex flex-col border-l border-base-300 pl-1.5">
        {#each links as l (l.href)}
          {@const active = isActiveLink($location, l)}
          <li>
            <a
              href={link(l.href)}
              class="flex items-center gap-1.5 rounded px-2 py-1 text-sm transition-colors {active
                ? 'bg-primary/10 font-medium text-primary'
                : 'text-base-content/70 hover:bg-base-300/50 hover:text-base-content'}"
              aria-current={active ? 'page' : undefined}
            >
              <span
                class="select-none {active
                  ? 'text-primary'
                  : 'text-transparent'}"
                aria-hidden="true">&gt;</span
              >
              <span class="truncate">{l.label}</span>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/snippet}

<aside
  class="flex h-full min-h-screen w-64 flex-col border-r border-base-300 bg-base-100"
>
  <!-- Brand: the command prompt -->
  <a
    href={link('/')}
    class="flex items-center gap-2.5 border-b border-base-300 px-4 py-3.5 transition-colors hover:bg-base-300/30"
  >
    <span
      class="nw-glow grid h-7 w-7 place-items-center rounded-xs bg-primary/15 font-bold text-primary"
      aria-hidden="true">◈</span
    >
    <span
      class="font-mono text-base font-semibold tracking-tight text-base-content"
      >northwatch</span
    >
  </a>

  <nav class="flex-1 overflow-y-auto px-2 py-3">
    {#each sections as section (section.key)}
      {@render group(section.key, section.label, section.links)}
    {/each}

    <div class="my-2 flex items-center gap-2 px-2">
      <span
        class="font-mono text-2xs tracking-widest text-base-content/30 uppercase"
        >Databases</span
      >
      <span class="h-px flex-1 bg-base-300"></span>
    </div>

    {#each dbGroups as db (db.key)}
      {@render group(db.key, db.label, db.links)}
    {/each}
  </nav>
</aside>
