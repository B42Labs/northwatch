<script lang="ts">
  import Sidebar from './Sidebar.svelte';
  import Navbar from './Navbar.svelte';
  import type { Snippet } from 'svelte';
  import { sidebarOpen } from '../../lib/sidebarStore';

  let { children }: { children: Snippet } = $props();
</script>

<div class="drawer" class:lg:drawer-open={$sidebarOpen}>
  <input id="sidebar-toggle" type="checkbox" class="drawer-toggle" />

  <div class="drawer-content flex flex-col">
    <Navbar />
    <main class="flex-1 overflow-y-auto p-4 lg:p-6">
      {@render children()}
    </main>
  </div>

  <div class="drawer-side z-20">
    <label
      for="sidebar-toggle"
      aria-label="close sidebar"
      class="drawer-overlay"
    ></label>
    <Sidebar />
  </div>
</div>
