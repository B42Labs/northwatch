<script lang="ts">
  import { theme } from '../../lib/stores';

  function toggle() {
    theme.update((t) =>
      t === 'northwatch-dark' ? 'northwatch-light' : 'northwatch-dark',
    );
  }

  let isDark = $derived($theme === 'northwatch-dark');
</script>

<button
  type="button"
  class="btn btn-square btn-ghost btn-sm font-mono text-base"
  onclick={toggle}
  title={isDark ? 'Switch to light' : 'Switch to dark'}
  aria-label="Toggle theme"
>
  {isDark ? '☾' : '☀'}
</button>
