<script lang="ts">
  import { push } from '../../lib/router';
  import ThemeToggle from './ThemeToggle.svelte';
  import ConnectionStatus from '../ui/ConnectionStatus.svelte';

  let searchQuery = $state('');

  function handleSearch(e: Event) {
    e.preventDefault();
    if (searchQuery.trim()) {
      push(`/search?q=${encodeURIComponent(searchQuery.trim())}`);
    }
  }
</script>

<div class="navbar gap-2 border-b border-base-300 bg-base-100 px-4">
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
