<script module lang="ts">
  import type { WsEvent } from '../../lib/websocket';

  // A live WebSocket event tagged with a stable, monotonic sequence id so it
  // can be keyed in {#each} and adapted into an EventRecord for the detail panel.
  export interface StreamEvent extends WsEvent {
    seq: number;
  }
</script>

<script lang="ts">
  import { actionVariant, actionGlyph } from '../../lib/status';
  import Badge from '../ui/Badge.svelte';

  interface Props {
    events: StreamEvent[];
    onSelect: (event: StreamEvent) => void;
    onClear: () => void;
  }

  let { events, onSelect, onClear }: Props = $props();

  let collapsed = $state(false);

  // Mouse-resizable size. The panel is pinned to the bottom-right corner, so
  // the resize grip lives at the top-left and dragging it up/left grows the
  // panel. Sizes are clamped to the graph container so it can't overflow.
  let panelEl: HTMLDivElement | undefined = $state();
  let width = $state(320);
  let height = $state(320);

  function startResize(e: PointerEvent) {
    e.preventDefault();
    const startX = e.clientX;
    const startY = e.clientY;
    const startW = width;
    const startH = height;
    const parent = panelEl?.parentElement;
    const maxW = parent ? parent.clientWidth - 24 : Infinity;
    const maxH = parent ? parent.clientHeight - 24 : Infinity;

    function onMove(ev: PointerEvent) {
      width = Math.max(240, Math.min(maxW, startW + (startX - ev.clientX)));
      height = Math.max(140, Math.min(maxH, startH + (startY - ev.clientY)));
    }
    function onUp() {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
      document.body.style.userSelect = '';
    }
    document.body.style.userSelect = 'none';
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  }

  // Keyboard resize: arrow keys grow/shrink the panel in the same directions a
  // pointer drag would, clamped to the same bounds as the drag handler.
  function resizeByKey(e: KeyboardEvent) {
    const step = 24;
    const parent = panelEl?.parentElement;
    const maxW = parent ? parent.clientWidth - 24 : Infinity;
    const maxH = parent ? parent.clientHeight - 24 : Infinity;
    switch (e.key) {
      case 'ArrowLeft':
        width = Math.min(maxW, width + step);
        break;
      case 'ArrowRight':
        width = Math.max(240, width - step);
        break;
      case 'ArrowUp':
        height = Math.min(maxH, height + step);
        break;
      case 'ArrowDown':
        height = Math.max(140, height - step);
        break;
      default:
        return;
    }
    e.preventDefault();
  }

  function eventTime(e: StreamEvent): string {
    return new Date(e.ts).toLocaleTimeString();
  }
</script>

<div
  bind:this={panelEl}
  class="absolute right-3 bottom-3 z-30 flex flex-col overflow-hidden rounded border border-base-300 bg-base-100/95 shadow-xl backdrop-blur"
  style="width: {width}px; max-width: calc(100% - 1.5rem); max-height: calc(100% - 1.5rem);{collapsed
    ? ''
    : ` height: ${height}px;`}"
>
  {#if !collapsed}
    <!-- Resize grip (top-left corner) -->
    <button
      type="button"
      class="absolute top-0 left-0 z-10 h-4 w-4 cursor-nwse-resize border-0 bg-transparent p-0"
      onpointerdown={startResize}
      onkeydown={resizeByKey}
      aria-label="Resize event stream panel (arrow keys)"
      title="Drag to resize"
    >
      <span
        class="absolute top-1 left-1 h-2 w-2 border-t-2 border-l-2 border-base-content/30"
      ></span>
    </button>
  {/if}

  <!-- Header -->
  <div
    class="flex items-center gap-2 border-b border-base-300 bg-base-200/60 px-2.5 py-1.5"
  >
    <span class="relative flex h-2 w-2" aria-hidden="true">
      <span
        class="absolute inline-flex h-full w-full animate-ping rounded-full bg-success opacity-75"
      ></span>
      <span class="relative inline-flex h-2 w-2 rounded-full bg-success"></span>
    </span>
    <span
      class="font-mono text-2xs tracking-wider text-base-content/70 uppercase"
      >Event Stream</span
    >
    <span class="font-mono text-2xs text-base-content/40 tabular-nums"
      >{events.length}</span
    >
    <div class="ml-auto flex items-center gap-0.5">
      {#if events.length > 0}
        <button
          class="btn btn-ghost px-1.5 font-mono text-2xs text-base-content/60 normal-case btn-xs"
          onclick={onClear}
          title="Clear stream">clear</button
        >
      {/if}
      <button
        class="btn btn-square btn-ghost btn-xs"
        onclick={() => (collapsed = !collapsed)}
        aria-label={collapsed ? 'Expand event stream' : 'Collapse event stream'}
        title={collapsed ? 'Expand' : 'Collapse'}
      >
        <span
          class="inline-block font-mono text-xs transition-transform {collapsed
            ? ''
            : 'rotate-180'}">&#9650;</span
        >
      </button>
    </div>
  </div>

  <!-- Body -->
  {#if !collapsed}
    <div class="flex-1 overflow-auto">
      {#if events.length === 0}
        <div class="px-2.5 py-3 font-mono text-2xs text-base-content/40">
          <span class="text-base-content/30">//</span> waiting for changes…
        </div>
      {:else}
        <table class="table-pin-rows table w-full table-xs font-mono">
          <thead>
            <tr>
              <th
                class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                >Time</th
              >
              <th
                class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                >Type</th
              >
              <th
                class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                >Table</th
              >
              <th
                class="bg-base-200 text-2xs tracking-wider text-base-content/55 uppercase"
                >UUID</th
              >
            </tr>
          </thead>
          <tbody>
            {#each events as evt (evt.seq)}
              <tr
                class="animate-fade-in cursor-pointer border-base-300/60 hover:bg-base-300/40"
                onclick={() => onSelect(evt)}
                title="Show details"
              >
                <td class="text-2xs whitespace-nowrap text-base-content/55"
                  >{eventTime(evt)}</td
                >
                <td>
                  <Badge
                    text={evt.type}
                    variant={actionVariant(evt.type)}
                    glyph={actionGlyph(evt.type)}
                  />
                </td>
                <td class="text-2xs whitespace-nowrap text-base-content/80"
                  >{evt.table}</td
                >
                <td class="text-2xs whitespace-nowrap text-base-content/55"
                  >{evt.uuid.slice(0, 8)}</td
                >
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  {/if}
</div>

<style>
  @keyframes fade-in {
    from {
      background-color: color-mix(
        in oklab,
        var(--color-secondary) 20%,
        transparent
      );
    }
    to {
      background-color: transparent;
    }
  }

  .animate-fade-in {
    animation: fade-in 1.5s ease-out;
  }
</style>
