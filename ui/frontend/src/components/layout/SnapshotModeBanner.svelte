<script lang="ts">
  import { snapshotMode, snapshotInfo } from '../../lib/capabilitiesStore';
  import type { SnapshotInfo } from '../../lib/api';

  let detail = $derived(buildDetail($snapshotInfo));

  function buildDetail(info: SnapshotInfo | null): string {
    if (info?.createdAt) {
      const d = new Date(info.createdAt);
      const when = Number.isNaN(d.getTime())
        ? info.createdAt
        : d.toLocaleString();
      return ` (captured ${when})`;
    }
    return '';
  }
</script>

{#if $snapshotMode}
  <div
    role="alert"
    class="flex items-center justify-center gap-2 border-b px-4 py-1.5 font-mono text-2xs tracking-wider uppercase"
    style="color: oklch(0.72 0.18 55); background-color: oklch(0.72 0.18 55 / 0.13); border-color: oklch(0.72 0.18 55 / 0.3);"
  >
    <span class="font-bold" aria-hidden="true">◉</span>
    <span>snapshot mode — viewing an offline copy, not live OVN{detail}</span>
  </div>
{/if}
