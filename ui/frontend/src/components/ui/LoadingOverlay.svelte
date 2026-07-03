<script lang="ts">
  // Full-screen blocking overlay shown during a slow action so the user can't
  // trigger it again. Renders nothing when not visible.
  let {
    show = false,
    message = 'loading',
    minMs = 400,
  }: { show?: boolean; message?: string; minMs?: number } = $props();

  // Keep the overlay up for at least minMs so even a fast operation is
  // perceptible. `wasShown` is intentionally non-reactive so the effect depends
  // only on `show` (and minMs) and cannot loop.
  let visible = $state(false);
  let shownAt = 0;
  let wasShown = false;

  $effect(() => {
    if (show) {
      if (!wasShown) {
        shownAt = performance.now();
        wasShown = true;
      }
      visible = true;
      return;
    }
    if (!wasShown) return;
    wasShown = false;
    const remaining = Math.max(0, minMs - (performance.now() - shownAt));
    const t = setTimeout(() => {
      visible = false;
    }, remaining);
    return () => clearTimeout(t);
  });

  // Render at <body> so the overlay is never trapped by an ancestor's stacking
  // context, transform, or overflow (e.g. when triggered from the navbar).
  function portal(node: HTMLElement) {
    document.body.appendChild(node);
    return { destroy: () => node.remove() };
  }
</script>

{#if visible}
  <div
    use:portal
    class="fixed inset-0 z-[1000] flex items-center justify-center bg-black/60 backdrop-blur-xs"
    role="status"
    aria-live="assertive"
    aria-busy="true"
  >
    <div
      class="flex items-center gap-4 rounded-lg border border-primary/30 bg-base-100 px-6 py-5 shadow-2xl"
    >
      <span class="nw-radar" aria-hidden="true"></span>
      <span class="nw-glow font-mono text-sm text-primary">{message}</span>
    </div>
  </div>
{/if}
