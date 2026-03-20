<script lang="ts">
  import { wsConnectionState } from '../../lib/eventStore';

  let state = $derived($wsConnectionState);

  let label = $derived(
    state === 'connected'
      ? 'Live'
      : state === 'connecting'
        ? 'Connecting'
        : 'Disconnected',
  );

  let colorClass = $derived(
    state === 'connected'
      ? 'bg-success'
      : state === 'connecting'
        ? 'bg-warning'
        : 'bg-error',
  );
</script>

<div class="flex items-center gap-1.5" title="WebSocket: {label}">
  <span class="h-2 w-2 rounded-full {colorClass}"></span>
  <span class="text-xs text-base-content/60">{label}</span>
</div>
