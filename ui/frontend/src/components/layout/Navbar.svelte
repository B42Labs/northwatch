<script lang="ts">
  import { push } from '../../lib/router';
  import ThemeToggle from './ThemeToggle.svelte';
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

<div class="navbar gap-2 border-b border-base-300 bg-base-100 px-4">
  <!-- Mobile hamburger (small screens) -->
  <div class="flex-none lg:hidden">
    <label for="sidebar-toggle" class="btn btn-square btn-ghost btn-sm">
      <svg
        xmlns="http://www.w3.org/2000/svg"
        fill="none"
        viewBox="0 0 24 24"
        class="h-5 w-5 stroke-current"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M4 6h16M4 12h16M4 18h16"
        />
      </svg>
    </label>
  </div>

  <!-- Desktop sidebar toggle (large screens) -->
  <div class="hidden flex-none lg:block">
    <button
      class="btn btn-square btn-ghost btn-sm"
      onclick={toggleSidebar}
      title={$sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        fill="none"
        viewBox="0 0 24 24"
        class="h-5 w-5 stroke-current"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          stroke-width="2"
          d="M4 6h16M4 12h16M4 18h16"
        />
      </svg>
    </button>
  </div>

  <div class="flex-1">
    <form onsubmit={handleSearch} class="w-full max-w-md">
      <input
        type="text"
        placeholder="Search by IP, MAC, UUID, or name..."
        class="input input-sm input-bordered w-full"
        bind:value={searchQuery}
      />
    </form>
  </div>

  <div class="flex-none">
    <ConnectionStatus />
  </div>

  <div class="flex-none">
    <ThemeToggle />
  </div>
</div>
