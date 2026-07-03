<script lang="ts">
  import { push } from '../../lib/router';
  import ThemeToggle from './ThemeToggle.svelte';
  import ClusterSwitcher from './ClusterSwitcher.svelte';
  import ConnectionStatus from '../ui/ConnectionStatus.svelte';
  import { sidebarOpen } from '../../lib/sidebarStore';

  let searchQuery = $state('');

  function handleSearch(e: Event) {
    e.preventDefault();
    if (searchQuery.trim()) {
      push(`/search?q=${encodeURIComponent(searchQuery.trim())}`);
    }
  }

  function toggleSidebar() {
    sidebarOpen.update((v) => !v);
  }
</script>

<header
  class="sticky top-0 z-20 flex h-12 items-center gap-2 border-b border-base-300 bg-base-100/95 px-3 backdrop-blur"
>
  <!-- Mobile drawer trigger -->
  <label
    for="sidebar-toggle"
    class="btn btn-square btn-ghost font-mono text-lg btn-sm lg:hidden"
    aria-label="Open navigation">≡</label
  >

  <!-- Desktop sidebar collapse -->
  <button
    type="button"
    class="btn hidden btn-square btn-ghost font-mono text-lg btn-sm lg:inline-flex"
    onclick={toggleSidebar}
    title={$sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}
    aria-label="Toggle sidebar">≡</button
  >

  <!-- Global search, command-bar style -->
  <form onsubmit={handleSearch} class="min-w-0 flex-1">
    <label class="relative block w-full max-w-lg">
      <span
        class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 font-mono text-sm text-primary select-none"
        aria-hidden="true">⌕</span
      >
      <input
        type="text"
        placeholder="search by ip, mac, uuid, or name…"
        class="input w-full bg-base-200/60 pl-8 font-mono input-sm"
        bind:value={searchQuery}
        aria-label="Search"
      />
    </label>
  </form>

  <div class="flex flex-none items-center gap-2">
    <ClusterSwitcher />
    <span class="hidden h-4 w-px bg-base-300 sm:block"></span>
    <ConnectionStatus />
    <ThemeToggle />
  </div>
</header>
