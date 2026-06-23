<script lang="ts">
  import { wsConnectionState } from '../../lib/eventStore';
  import { connLabel, type ConnState } from '../../lib/status';

  let state = $derived($wsConnectionState as ConnState);

  let dotClass = $derived(
    state === 'connected'
      ? 'nw-dot nw-dot-live'
      : state === 'connecting'
        ? 'nw-dot bg-warning'
        : 'nw-dot bg-error',
  );
</script>

<div
  class="flex items-center gap-1.5 font-mono text-2xs uppercase tracking-wider text-base-content/55"
  title="WebSocket: {connLabel(state)}"
>
  <span class={dotClass}></span>
  <span>{connLabel(state)}</span>
</div>
