<script lang="ts">
  import { wsConnectionState } from '../../lib/eventStore';
  import { snapshotMode, snapshotInfo } from '../../lib/capabilitiesStore';
  import { connLabel, type ConnState } from '../../lib/status';
  import type { SnapshotInfo } from '../../lib/api';

  let state = $derived($wsConnectionState as ConnState);

  // In snapshot mode the data source is a captured file, not a live OVN
  // deployment — surface that as a distinct amber/orange "snapshot" state.
  // A genuinely broken transport still reads as "offline".
  let snapshot = $derived($snapshotMode && state !== 'disconnected');

  let dotClass = $derived(
    snapshot
      ? 'nw-dot nw-dot-snapshot'
      : state === 'connected'
        ? 'nw-dot nw-dot-live'
        : state === 'connecting'
          ? 'nw-dot bg-warning'
          : 'nw-dot bg-error',
  );

  let label = $derived(snapshot ? 'snapshot' : connLabel(state));

  let tip = $derived(
    snapshot ? snapshotTip($snapshotInfo) : `WebSocket: ${connLabel(state)}`,
  );

  let textClass = $derived(snapshot ? 'text-warning' : 'text-base-content/55');

  function snapshotTip(info: SnapshotInfo | null): string {
    const parts = ['Offline snapshot — not live OVN data'];
    if (info?.createdAt) {
      const d = new Date(info.createdAt);
      parts.push(
        `captured ${Number.isNaN(d.getTime()) ? info.createdAt : d.toLocaleString()}`,
      );
    }
    if (info?.nbAddr) parts.push(`source ${info.nbAddr}`);
    return parts.join(' · ');
  }
</script>

<div
  class="flex items-center gap-1.5 font-mono text-2xs uppercase tracking-wider {textClass}"
  title={tip}
>
  <span class={dotClass}></span>
  <span>{label}</span>
</div>
