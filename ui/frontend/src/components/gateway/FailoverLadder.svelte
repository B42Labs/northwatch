<script lang="ts">
  import * as d3 from 'd3';
  import type { GatewayHealth, GatewayMember } from '../../lib/api';

  let { gateway }: { gateway: GatewayHealth } = $props();

  let svgRef = $state<SVGSVGElement | undefined>(undefined);

  const GREEN = '#4ade80';
  const AMBER = '#fbbf24';
  const RED = '#f87171';
  const GRAY = '#6b7280';
  const MUTED = '#9ca3af';

  function overallColor(o: string): string {
    return o === 'error' ? RED : o === 'warning' ? AMBER : GREEN;
  }

  // The "actual" path is solid; the elected master (if different) is a dashed
  // green hint of where traffic should go; standbys are faint dashed gray.
  function edgeStyle(m: GatewayMember): {
    stroke: string;
    width: number;
    dash: string | null;
  } {
    if (m.active)
      return { stroke: overallColor(gateway.overall), width: 2.5, dash: null };
    if (m.desired) return { stroke: GREEN, width: 1.75, dash: '6,4' };
    return { stroke: GRAY, width: 1, dash: '3,4' };
  }

  function nodeStroke(m: GatewayMember): string {
    if (m.active) return overallColor(gateway.overall);
    if (m.desired) return GREEN;
    if (!m.alive) return RED;
    return GRAY;
  }

  function tags(m: GatewayMember): { text: string; color: string }[] {
    const out: { text: string; color: string }[] = [];
    if (m.active)
      out.push({ text: 'ACTIVE', color: overallColor(gateway.overall) });
    if (m.desired) out.push({ text: 'DESIRED', color: GREEN });
    if (m.present && m.stale) out.push({ text: 'STALE', color: AMBER });
    if (!m.present) out.push({ text: 'DOWN', color: RED });
    if (m.present && !m.gateway_capable)
      out.push({ text: 'NOT-GW', color: AMBER });
    return out;
  }

  function draw() {
    if (!svgRef) return;
    const svg = d3.select(svgRef);
    svg.selectAll('*').remove();

    const width = svgRef.clientWidth || 640;
    const height = 250;
    svg.attr('viewBox', `0 0 ${width} ${height}`);

    const extY = 30;
    const crY = 116;
    const chY = 210;
    const cx = width / 2;

    let members: GatewayMember[] = [...gateway.members].sort(
      (a, b) => b.priority - a.priority,
    );
    // Unmanaged gateway (no HA group): synthesize the single pinned chassis.
    if (members.length === 0 && gateway.actual_chassis) {
      members = [
        {
          name: gateway.actual_chassis,
          priority: 0,
          present: true,
          stale: false,
          gateway_capable: true,
          alive: true,
          active: true,
          desired: false,
        },
      ];
    }

    const n = Math.max(members.length, 1);
    const slot = width / (n + 1);
    const boxW = Math.max(68, Math.min(140, slot - 14));
    const xs = members.map((_, i) => slot * (i + 1));

    // --- edges (drawn first, behind nodes) ---
    // external network -> cr-lrp
    svg
      .append('line')
      .attr('x1', cx)
      .attr('y1', extY + 18)
      .attr('x2', cx)
      .attr('y2', crY - 18)
      .attr('stroke', GRAY)
      .attr('stroke-width', 1.25);

    // cr-lrp -> each member
    members.forEach((m, i) => {
      const s = edgeStyle(m);
      const line = svg
        .append('line')
        .attr('x1', cx)
        .attr('y1', crY + 18)
        .attr('x2', xs[i])
        .attr('y2', chY - 22)
        .attr('stroke', s.stroke)
        .attr('stroke-width', s.width);
      if (s.dash) line.attr('stroke-dasharray', s.dash);
    });

    // --- node helper ---
    const box = (
      x: number,
      y: number,
      w: number,
      h: number,
      stroke: string,
      strokeW = 1.5,
    ) =>
      svg
        .append('rect')
        .attr('x', x - w / 2)
        .attr('y', y - h / 2)
        .attr('width', w)
        .attr('height', h)
        .attr('rx', 4)
        .attr('fill', 'none')
        .attr('stroke', stroke)
        .attr('stroke-width', strokeW);

    const label = (
      x: number,
      y: number,
      text: string,
      opts: { color?: string; size?: number; weight?: string } = {},
    ) =>
      svg
        .append('text')
        .attr('x', x)
        .attr('y', y)
        .attr('text-anchor', 'middle')
        .attr('dominant-baseline', 'middle')
        .attr('font-family', 'monospace')
        .attr('font-size', opts.size ?? 11)
        .attr('font-weight', opts.weight ?? 'normal')
        .attr('fill', opts.color ?? 'currentColor')
        .text(text);

    // external network node
    box(cx, extY, 220, 30, GRAY);
    label(cx, extY, gateway.external_networks?.[0] ?? 'external network', {
      color: MUTED,
    });

    // cr-lrp node
    box(cx, crY, 240, 34, overallColor(gateway.overall), 1.75);
    label(cx, crY - 5, gateway.cr_port, { weight: 'bold', size: 11 });
    label(cx, crY + 9, gateway.router_name ?? '', { color: MUTED, size: 9.5 });

    // chassis member nodes
    members.forEach((m, i) => {
      const x = xs[i];
      const st = nodeStroke(m);
      box(x, chY, boxW, 40, st, m.active || m.desired ? 2 : 1.25);
      label(x, chY - 9, m.name, {
        weight: 'bold',
        size: 10.5,
        color: m.alive ? 'currentColor' : RED,
      });
      label(x, chY + 3, `prio ${m.priority}`, { color: MUTED, size: 9 });
      const ts = tags(m);
      if (ts.length) {
        // Render up to two tags on one line.
        const shown = ts.slice(0, 2);
        const txt = shown.map((t) => t.text).join(' · ');
        label(x, chY + 14, txt, { color: shown[0].color, size: 8 });
      }
    });
  }

  $effect(() => {
    // Track the gateway prop so the ladder redraws on data changes.
    void gateway;
    if (!svgRef) return;
    draw();
    const ro = new ResizeObserver(() => draw());
    ro.observe(svgRef);
    return () => ro.disconnect();
  });
</script>

<div
  class="w-full overflow-hidden rounded border border-base-300 bg-base-200/40"
>
  <svg
    bind:this={svgRef}
    class="block h-[250px] w-full text-base-content"
    role="img"
    aria-label="Gateway failover ladder for {gateway.cr_port}"
  ></svg>
</div>
