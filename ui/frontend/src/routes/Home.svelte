<script lang="ts">
  import { link } from '../lib/router';
  import { capabilities, writeEnabled } from '../lib/capabilitiesStore';
  import { navSections } from '../lib/nav';
  import PageContainer from '../components/ui/PageContainer.svelte';
  import Badge from '../components/ui/Badge.svelte';

  let sections = $derived(
    navSections.filter((s) => !s.requiresWrite || $writeEnabled),
  );
</script>

<PageContainer width="wide">
  <!-- Hero: a terminal window. The most characteristic thing on the page. -->
  <section class="mb-8 overflow-hidden rounded border border-base-300 bg-base-100">
    <div class="flex items-center gap-2 border-b border-base-300 bg-base-200/50 px-3 py-2">
      <span class="h-2.5 w-2.5 rounded-full bg-error/70"></span>
      <span class="h-2.5 w-2.5 rounded-full bg-warning/70"></span>
      <span class="h-2.5 w-2.5 rounded-full bg-success/70"></span>
      <span class="ml-2 font-mono text-2xs uppercase tracking-widest text-base-content/40"
        >northwatch — console</span
      >
    </div>
    <div class="px-5 py-6 sm:px-8 sm:py-8">
      <div class="flex items-center gap-2 font-mono text-sm text-base-content/55">
        <span class="text-primary">northwatch:~$</span>
        <span>status</span>
      </div>
      <h1
        class="mt-3 flex items-center font-mono text-3xl font-bold tracking-tight text-base-content sm:text-4xl"
      >
        <span class="text-primary nw-glow">◈</span>
        <span class="ml-2">northwatch</span>
        <span class="nw-cursor" aria-hidden="true"></span>
      </h1>
      <p class="mt-3 max-w-2xl font-prose text-sm leading-relaxed text-base-content/65">
        A read-only console for browsing, debugging, and monitoring OVN
        deployments — correlating the Northbound intent and Southbound realized
        state of your virtual network in one place.
      </p>
      <div class="mt-4 flex flex-wrap items-center gap-1.5">
        <Badge
          text={$writeEnabled ? 'write enabled' : 'read-only'}
          variant={$writeEnabled ? 'warning' : 'success'}
          glyph={$writeEnabled ? '!' : '✓'}
        />
        {#each $capabilities as cap (cap)}
          <Badge text={cap} variant="ghost" />
        {/each}
      </div>
    </div>
  </section>

  <!-- Module launcher, driven by the shared nav config. -->
  <div class="mb-3 flex items-center gap-2">
    <span class="font-mono text-2xs uppercase tracking-widest text-base-content/40"
      >modules</span
    >
    <span class="h-px flex-1 bg-base-300"></span>
  </div>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
    {#each sections as section (section.key)}
      <section class="flex flex-col rounded border border-base-300 bg-base-100">
        <header class="border-b border-base-300 px-4 py-2.5">
          <h2 class="font-mono text-sm font-semibold text-base-content">
            {section.label}
          </h2>
          <p class="mt-0.5 font-prose text-xs text-base-content/50">
            {section.description}
          </p>
        </header>
        <ul class="flex flex-col p-1.5">
          {#each section.links as l (l.href)}
            <li>
              <a
                href={link(l.href)}
                class="group flex items-center gap-1.5 rounded px-2.5 py-1.5 font-mono text-sm text-base-content/75 transition-colors hover:bg-base-300/50 hover:text-primary"
              >
                <span class="select-none text-transparent group-hover:text-primary"
                  >&gt;</span
                >
                <span class="truncate">{l.label}</span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  </div>
</PageContainer>
