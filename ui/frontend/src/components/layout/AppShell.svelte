<script lang="ts">
  import { onMount } from 'svelte';
  import Sidebar from './Sidebar.svelte';
  import Navbar from './Navbar.svelte';
  import WriteModeBanner from './WriteModeBanner.svelte';
  import type { Snippet } from 'svelte';
  import { sidebarOpen } from '../../lib/sidebarStore';
  import { loadClusters } from '../../lib/clusterStore';
  import { loadCapabilities } from '../../lib/capabilitiesStore';

  let { children }: { children: Snippet } = $props();

  onMount(() => {
    loadClusters();
    loadCapabilities();
  });
</script>

<div class="drawer" class:lg:drawer-open={$sidebarOpen}>
  <input id="sidebar-toggle" type="checkbox" class="drawer-toggle" />

  <div class="drawer-content flex min-h-screen flex-col">
    <WriteModeBanner />
    <Navbar />
    <main class="flex-1 overflow-y-auto p-4 lg:p-6">
      {@render children()}
    </main>
  </div>

  <div class="drawer-side z-30">
    <label
      for="sidebar-toggle"
      aria-label="close sidebar"
      class="drawer-overlay"
    ></label>
    <Sidebar />
  </div>
</div>

<!-- Signature: subtle CRT scanlines + vignette over the whole console.
     Both are pointer-events:none and hidden under prefers-reduced-motion. -->
<div class="nw-vignette" aria-hidden="true"></div>
<div class="nw-scanlines" aria-hidden="true"></div>
