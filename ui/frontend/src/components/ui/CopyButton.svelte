<script lang="ts">
  import { onDestroy } from 'svelte';
  import { copyText } from '../../lib/copyView';

  // A button that puts text on the clipboard and says so. The UI has no toast,
  // so the confirmation is the label itself, swapped back after a moment.
  let {
    text,
    label,
    ariaLabel,
    disabled = false,
    class: extra = 'btn border-base-300 btn-ghost btn-xs',
  }: {
    /** Builds the text to copy. Called at click time, never earlier. */
    text: () => string;
    label: string;
    ariaLabel: string;
    disabled?: boolean;
    class?: string;
  } = $props();

  const REVERT_MS = 1500;

  let phase = $state<'idle' | 'copied' | 'failed'>('idle');
  let busy = $state(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  onDestroy(() => {
    if (timer) clearTimeout(timer);
  });

  function settle(next: 'copied' | 'failed') {
    phase = next;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      phase = 'idle';
      timer = null;
    }, REVERT_MS);
  }

  async function run() {
    // Guarded rather than left to the disabled attribute, so a synthetic click
    // cannot slip past either.
    if (busy || disabled) return;
    busy = true;
    try {
      // text() serializes the view and can throw on its own (a cyclic object),
      // so it sits inside the same try as the clipboard write.
      await copyText(text());
      settle('copied');
    } catch (err) {
      console.error('copy view:', err);
      settle('failed');
    } finally {
      busy = false;
    }
  }

  let shown = $derived(
    phase === 'copied' ? 'Copied' : phase === 'failed' ? 'Failed' : label,
  );
</script>

<button
  type="button"
  class={extra}
  aria-label={ariaLabel}
  disabled={disabled || busy}
  onclick={run}
>
  {shown}
</button>
