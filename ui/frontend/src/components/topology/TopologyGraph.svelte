<script lang="ts">
  import * as d3 from 'd3';
  import type { TopologyNode, TopologyEdge } from '../../lib/api';
  import { push } from '../../lib/router';

  let {
    nodes,
    edges,
    searchQuery = '',
    relayoutKey = 0,
  }: {
    nodes: TopologyNode[];
    edges: TopologyEdge[];
    searchQuery?: string;
    relayoutKey?: number;
  } = $props();

  let svgElement: SVGSVGElement;
  let containerEl: HTMLDivElement;

  // Hover info card state
  let hoveredNode: TopologyNode | null = $state(null);
  let infoPos = $state({ x: 0, y: 0 });

  const palette: Record<
    string,
    { fill: string; stroke: string; glow: string; label: string } | string
  > = {
    switch: {
      fill: '#2563eb',
      stroke: '#1d4ed8',
      glow: 'rgba(37,99,235,0.3)',
      label: 'Logical Switch',
    },
    router: {
      fill: '#059669',
      stroke: '#047857',
      glow: 'rgba(5,150,105,0.3)',
      label: 'Logical Router',
    },
    chassis: {
      fill: '#475569',
      stroke: '#334155',
      glow: 'rgba(71,85,105,0.2)',
      label: 'Chassis',
    },
    gateway: {
      fill: '#7c3aed',
      stroke: '#6d28d9',
      glow: 'rgba(124,58,237,0.3)',
      label: 'Gateway Chassis',
    },
    'vm-port': {
      fill: '#f59e0b',
      stroke: '#d97706',
      glow: 'rgba(245,158,11,0.25)',
      label: 'VM Port',
    },
    edgeLogical: '#64748b',
    edgeBinding: '#94a3b8',
    bg: '#0f172a',
    text: '#e2e8f0',
    textMuted: '#94a3b8',
  };

  // Build info rows for the hover card
  function infoRows(d: TopologyNode): { key: string; value: string }[] {
    const rows: { key: string; value: string }[] = [];
    rows.push({ key: 'UUID', value: d.id });

    if (d.type === 'vm-port' && d.metadata) {
      if (d.metadata.cidrs) rows.push({ key: 'IP', value: d.metadata.cidrs });
      if (d.metadata.port_fip)
        rows.push({ key: 'Floating IP', value: d.metadata.port_fip });
      if (d.metadata.mac) rows.push({ key: 'MAC', value: d.metadata.mac });
      if (d.metadata.host_id)
        rows.push({ key: 'Host', value: d.metadata.host_id });
      if (d.metadata.device_owner)
        rows.push({ key: 'Owner', value: d.metadata.device_owner });
      if (d.metadata.device_id)
        rows.push({ key: 'Device', value: d.metadata.device_id });
      if (d.metadata.network_name)
        rows.push({
          key: 'Network',
          value: shortLabel(d.metadata.network_name),
        });
      if (d.metadata.up != null)
        rows.push({
          key: 'Status',
          value: d.metadata.up === 'true' ? 'Up' : 'Down',
        });
    } else {
      // Switch / Router / Chassis — show full name
      if (d.type === 'chassis' && d.metadata?.role === 'gateway') {
        rows.push({ key: 'Role', value: 'Gateway (External Network)' });
      }
      const m = d.label.match(/^neutron-(.+)/);
      if (m) rows.push({ key: 'Neutron ID', value: m[1] });
      // Count connections
      const conns = edges.filter((e) => e.source === d.id || e.target === d.id);
      const logical = conns.filter(
        (e) => e.type === 'router-port' || e.type === 'patch',
      ).length;
      const bindings = conns.filter((e) => e.type === 'binding').length;
      const vms = conns.filter((e) => e.type === 'vm-binding').length;
      if (logical > 0)
        rows.push({ key: 'Connections', value: String(logical) });
      if (bindings > 0)
        rows.push({ key: 'Chassis bindings', value: String(bindings) });
      if (vms > 0) rows.push({ key: 'VM ports', value: String(vms) });
    }
    return rows;
  }

  function shortLabel(label: string): string {
    const m = label.match(/^neutron-([a-f0-9]{8})/);
    if (m) return m[1];
    return label;
  }

  function navigateToNode(node: TopologyNode): void {
    switch (node.type) {
      case 'switch':
        push(`/correlated/logical-switches/${node.id}`);
        break;
      case 'router':
        push(`/correlated/logical-routers/${node.id}`);
        break;
      case 'chassis':
        push(`/correlated/chassis/${node.id}`);
        break;
    }
  }

  function matchesSearch(d: TopologyNode, q: string): boolean {
    if (!q) return true;
    const lower = q.toLowerCase();
    if (d.label.toLowerCase().includes(lower)) return true;
    if (d.id.toLowerCase().includes(lower)) return true;
    if (d.metadata) {
      for (const v of Object.values(d.metadata)) {
        if (v.toLowerCase().includes(lower)) return true;
      }
    }
    return false;
  }

  $effect(() => {
    void nodes;
    void edges;
    void searchQuery;
    void relayoutKey;
    if (svgElement) renderGraph();
  });

  function renderGraph() {
    const svg = d3.select(svgElement);
    svg.selectAll('*').remove();
    hoveredNode = null;

    const width = svgElement.clientWidth || 900;
    const height = svgElement.clientHeight || 600;

    const hasVMs = nodes.some((n) => n.type === 'vm-port');

    // Defs
    const defs = svg.append('defs');

    const glowFilter = defs.append('filter').attr('id', 'glow');
    glowFilter
      .append('feGaussianBlur')
      .attr('stdDeviation', '3')
      .attr('result', 'blur');
    const feMerge = glowFilter.append('feMerge');
    feMerge.append('feMergeNode').attr('in', 'blur');
    feMerge.append('feMergeNode').attr('in', 'SourceGraphic');

    defs
      .append('marker')
      .attr('id', 'arrow-logical')
      .attr('viewBox', '0 0 10 6')
      .attr('refX', 10)
      .attr('refY', 3)
      .attr('markerWidth', 8)
      .attr('markerHeight', 6)
      .attr('orient', 'auto')
      .append('path')
      .attr('d', 'M0,0 L10,3 L0,6')
      .attr('fill', palette.edgeLogical);

    // Background + grid
    svg
      .append('rect')
      .attr('width', width)
      .attr('height', height)
      .attr('fill', palette.bg);

    const gridGroup = svg.append('g').attr('class', 'grid');
    const gridSize = 40;
    for (let x = 0; x < width; x += gridSize) {
      gridGroup
        .append('line')
        .attr('x1', x)
        .attr('y1', 0)
        .attr('x2', x)
        .attr('y2', height)
        .attr('stroke', '#1e293b')
        .attr('stroke-width', 0.5);
    }
    for (let y = 0; y < height; y += gridSize) {
      gridGroup
        .append('line')
        .attr('x1', 0)
        .attr('y1', y)
        .attr('x2', width)
        .attr('y2', y)
        .attr('stroke', '#1e293b')
        .attr('stroke-width', 0.5);
    }

    const g = svg.append('g');

    const zoom = d3
      .zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.2, 5])
      .on('zoom', (event) => {
        g.attr('transform', event.transform);
        gridGroup.attr('transform', event.transform);
      });
    svg.call(zoom);

    type SimNode = TopologyNode & d3.SimulationNodeDatum;
    type SimLink = d3.SimulationLinkDatum<SimNode> & { type: string };

    const simNodes: SimNode[] = nodes.map((n) => ({ ...n }));
    const nodeMap = new Map(simNodes.map((n) => [n.id, n]));

    const simLinks: SimLink[] = edges
      .filter((e) => nodeMap.has(e.source) && nodeMap.has(e.target))
      .map((e) => ({ source: e.source, target: e.target, type: e.type }));

    // Force simulation
    const simulation = d3
      .forceSimulation<SimNode>(simNodes)
      .force(
        'link',
        d3
          .forceLink<SimNode, SimLink>(simLinks)
          .id((d) => d.id)
          .distance((d) => {
            if (d.type === 'vm-binding') return 50;
            if (d.type === 'binding') return 100;
            return 150;
          })
          .strength((d) => {
            if (d.type === 'vm-binding') return 0.9;
            if (d.type === 'binding') return 0.6;
            return 0.4;
          }),
      )
      .force('charge', d3.forceManyBody().strength(-600))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force(
        'collision',
        d3.forceCollide<SimNode>((d) => (d.type === 'vm-port' ? 25 : 60)),
      );

    // --- Edges ---
    const bgLinks = simLinks.filter(
      (l) => l.type === 'binding' || l.type === 'vm-binding',
    );
    const fgLinks = simLinks.filter(
      (l) => l.type !== 'binding' && l.type !== 'vm-binding',
    );

    const bgEdge = g
      .append('g')
      .selectAll('path')
      .data(bgLinks)
      .join('path')
      .attr('fill', 'none')
      .attr('stroke', (d) =>
        d.type === 'vm-binding'
          ? palette['vm-port'].stroke
          : palette.edgeBinding,
      )
      .attr('stroke-width', 1)
      .attr('stroke-dasharray', '4 4')
      .attr('stroke-opacity', 0.4);

    const fgEdge = g
      .append('g')
      .selectAll('path')
      .data(fgLinks)
      .join('path')
      .attr('fill', 'none')
      .attr('stroke', palette.edgeLogical)
      .attr('stroke-width', 2)
      .attr('stroke-opacity', 0.8)
      .attr('marker-end', 'url(#arrow-logical)');

    // --- Nodes ---
    const node = g
      .append('g')
      .selectAll<SVGGElement, SimNode>('g')
      .data(simNodes)
      .join('g')
      .attr('cursor', 'pointer')
      .on('click', (_, d) => navigateToNode(d));

    node.call(
      d3
        .drag<SVGGElement, SimNode>()
        .on('start', (event, d) => {
          if (!event.active) simulation.alphaTarget(0.3).restart();
          d.fx = d.x;
          d.fy = d.y;
        })
        .on('drag', (event, d) => {
          d.fx = event.x;
          d.fy = event.y;
        })
        .on('end', (event, d) => {
          if (!event.active) simulation.alphaTarget(0);
          d.fx = null;
          d.fy = null;
        }),
    );

    // Glow ring
    function glowRadius(type: string): number {
      if (type === 'vm-port') return 14;
      if (type === 'chassis') return 28;
      return 24;
    }

    node
      .append('circle')
      .attr('r', (d) => glowRadius(d.type))
      .attr('fill', (d) => {
        if (d.type === 'chassis' && d.metadata?.role === 'gateway')
          return palette.gateway.glow;
        return palette[d.type]?.glow ?? 'transparent';
      })
      .attr('filter', 'url(#glow)');

    // Node shapes
    node.each(function (d) {
      const el = d3.select(this);
      const colors = palette[d.type];
      if (!colors) return;

      if (d.type === 'vm-port') {
        el.append('circle')
          .attr('r', 10)
          .attr('fill', colors.fill)
          .attr('stroke', colors.stroke)
          .attr('stroke-width', 1.5);
        const isUp = d.metadata?.up === 'true';
        el.append('circle')
          .attr('cx', 0)
          .attr('cy', 0)
          .attr('r', 3)
          .attr('fill', isUp ? '#22c55e' : '#ef4444');
      } else if (d.type === 'chassis') {
        const isGateway = d.metadata?.role === 'gateway';
        const c = isGateway ? palette.gateway : colors;
        el.append('rect')
          .attr('x', -24)
          .attr('y', -18)
          .attr('width', 48)
          .attr('height', 36)
          .attr('rx', 6)
          .attr('fill', c.fill)
          .attr('stroke', c.stroke)
          .attr('stroke-width', 2);
        if (isGateway) {
          // Globe/uplink icon for gateway chassis
          el.append('circle')
            .attr('cx', 0)
            .attr('cy', 0)
            .attr('r', 9)
            .attr('fill', 'none')
            .attr('stroke', '#c4b5fd')
            .attr('stroke-width', 1.2);
          el.append('ellipse')
            .attr('cx', 0)
            .attr('cy', 0)
            .attr('rx', 4)
            .attr('ry', 9)
            .attr('fill', 'none')
            .attr('stroke', '#c4b5fd')
            .attr('stroke-width', 1);
          el.append('line')
            .attr('x1', -9)
            .attr('y1', 0)
            .attr('x2', 9)
            .attr('y2', 0)
            .attr('stroke', '#c4b5fd')
            .attr('stroke-width', 1);
        } else {
          // Server rack icon for compute chassis
          el.append('line')
            .attr('x1', -14)
            .attr('y1', -6)
            .attr('x2', 14)
            .attr('y2', -6)
            .attr('stroke', '#94a3b8')
            .attr('stroke-width', 1);
          el.append('line')
            .attr('x1', -14)
            .attr('y1', 6)
            .attr('x2', 14)
            .attr('y2', 6)
            .attr('stroke', '#94a3b8')
            .attr('stroke-width', 1);
          el.append('circle')
            .attr('cx', 10)
            .attr('cy', -12)
            .attr('r', 2.5)
            .attr('fill', '#22c55e');
          el.append('circle')
            .attr('cx', 10)
            .attr('cy', 0)
            .attr('r', 2.5)
            .attr('fill', '#22c55e');
          el.append('circle')
            .attr('cx', 10)
            .attr('cy', 12)
            .attr('r', 2.5)
            .attr('fill', '#22c55e');
        }
      } else if (d.type === 'router') {
        el.append('path')
          .attr('d', 'M 0 -20 L 24 0 L 0 20 L -24 0 Z')
          .attr('fill', colors.fill)
          .attr('stroke', colors.stroke)
          .attr('stroke-width', 2);
        el.append('path')
          .attr('d', 'M -8 0 L 6 0 M 2 -4 L 6 0 L 2 4')
          .attr('fill', 'none')
          .attr('stroke', '#d1fae5')
          .attr('stroke-width', 1.5)
          .attr('stroke-linecap', 'round');
      } else {
        el.append('rect')
          .attr('x', -22)
          .attr('y', -16)
          .attr('width', 44)
          .attr('height', 32)
          .attr('rx', 5)
          .attr('fill', colors.fill)
          .attr('stroke', colors.stroke)
          .attr('stroke-width', 2);
        for (let i = -2; i <= 2; i++) {
          el.append('circle')
            .attr('cx', i * 7)
            .attr('cy', 0)
            .attr('r', 2)
            .attr('fill', '#bfdbfe');
        }
      }
    });

    // Label
    node
      .append('text')
      .text((d) => shortLabel(d.label))
      .attr('text-anchor', 'middle')
      .attr('dy', (d) => (d.type === 'vm-port' ? 22 : 34))
      .attr('font-size', (d) => (d.type === 'vm-port' ? '9px' : '11px'))
      .attr('font-weight', '500')
      .attr('fill', (d) =>
        d.type === 'vm-port' ? palette.textMuted : palette.text,
      )
      .attr('class', 'select-none');

    // Hover: show info card instead of native tooltip
    node
      .on('mouseenter', function (event: MouseEvent, d: SimNode) {
        const r = d.type === 'vm-port' ? 18 : 32;
        d3.select(this)
          .select('circle')
          .transition()
          .duration(200)
          .attr('r', r);

        // Position info card relative to container
        const rect = containerEl.getBoundingClientRect();
        infoPos = {
          x: event.clientX - rect.left + 16,
          y: event.clientY - rect.top - 8,
        };
        hoveredNode = d;
      })
      .on('mousemove', function (event: MouseEvent) {
        const rect = containerEl.getBoundingClientRect();
        infoPos = {
          x: event.clientX - rect.left + 16,
          y: event.clientY - rect.top - 8,
        };
      })
      .on('mouseleave', function (_, d: SimNode) {
        const r = glowRadius(d.type);
        d3.select(this)
          .select('circle')
          .transition()
          .duration(200)
          .attr('r', r);
        hoveredNode = null;
      });

    // Search dimming — reduce opacity for non-matching nodes and their edges
    if (searchQuery) {
      const matchIds = new Set(
        simNodes.filter((d) => matchesSearch(d, searchQuery)).map((d) => d.id),
      );
      node.attr('opacity', (d) => (matchIds.has(d.id) ? 1 : 0.15));
      bgEdge.attr('stroke-opacity', (d) => {
        const s = (d.source as SimNode).id;
        const t = (d.target as SimNode).id;
        return matchIds.has(s) || matchIds.has(t) ? 0.4 : 0.05;
      });
      fgEdge.attr('stroke-opacity', (d) => {
        const s = (d.source as SimNode).id;
        const t = (d.target as SimNode).id;
        return matchIds.has(s) || matchIds.has(t) ? 0.8 : 0.08;
      });
    }

    // Edge path
    function linkPath(d: SimLink): string {
      const s = d.source as SimNode;
      const t = d.target as SimNode;
      const dx = t.x! - s.x!;
      const dy = t.y! - s.y!;
      const dr = Math.sqrt(dx * dx + dy * dy) * 1.5;
      return `M${s.x},${s.y} A${dr},${dr} 0 0,1 ${t.x},${t.y}`;
    }

    // Tick
    simulation.on('tick', () => {
      bgEdge.attr('d', linkPath);
      fgEdge.attr('d', linkPath);
      node.attr('transform', (d) => `translate(${d.x},${d.y})`);
    });

    // --- Legend ---
    const legendItems: { label: string; color: string; shape: string }[] = [
      { label: 'Switch', color: palette.switch.fill, shape: 'rect' },
      { label: 'Router', color: palette.router.fill, shape: 'diamond' },
      { label: 'Chassis', color: palette.chassis.fill, shape: 'rect' },
      { label: 'Gateway', color: palette.gateway.fill, shape: 'rect' },
    ];
    if (hasVMs) {
      legendItems.push({
        label: 'VM Port',
        color: palette['vm-port'].fill,
        shape: 'circle',
      });
    }

    const legend = svg
      .append('g')
      .attr(
        'transform',
        `translate(16, ${height - legendItems.length * 24 - 20})`,
      );

    legend
      .append('rect')
      .attr('x', -8)
      .attr('y', -8)
      .attr('width', 105)
      .attr('height', legendItems.length * 24 + 16)
      .attr('rx', 6)
      .attr('fill', '#1e293b')
      .attr('fill-opacity', 0.8)
      .attr('stroke', '#334155')
      .attr('stroke-width', 1);

    legendItems.forEach((item, i) => {
      const row = legend
        .append('g')
        .attr('transform', `translate(0, ${i * 24})`);
      if (item.shape === 'diamond') {
        row
          .append('path')
          .attr('d', 'M 6 0 L 12 6 L 6 12 L 0 6 Z')
          .attr('fill', item.color);
      } else if (item.shape === 'circle') {
        row
          .append('circle')
          .attr('cx', 6)
          .attr('cy', 6)
          .attr('r', 5)
          .attr('fill', item.color);
      } else {
        row
          .append('rect')
          .attr('width', 12)
          .attr('height', 12)
          .attr('rx', 2)
          .attr('fill', item.color);
      }
      row
        .append('text')
        .attr('x', 20)
        .attr('y', 10)
        .attr('font-size', '11px')
        .attr('fill', palette.textMuted)
        .text(item.label);
    });
  }
</script>

<div bind:this={containerEl} class="relative h-full w-full">
  <svg
    bind:this={svgElement}
    class="h-full w-full"
    style="background: {palette.bg};"
  ></svg>

  {#if hoveredNode}
    {@const isGw =
      hoveredNode.type === 'chassis' &&
      hoveredNode.metadata?.role === 'gateway'}
    {@const hdrColor =
      (isGw ? palette.gateway : palette[hoveredNode.type])?.fill ?? '#6b7280'}
    {@const hdrLabel = isGw
      ? 'Gateway Chassis'
      : (palette[hoveredNode.type]?.label ?? hoveredNode.type)}
    <div
      class="pointer-events-none absolute z-50"
      style="left: {infoPos.x}px; top: {infoPos.y}px;"
    >
      <div
        class="rounded-lg border border-slate-600 bg-slate-800 px-3 py-2 shadow-xl"
        style="min-width: 340px;"
      >
        <!-- Header -->
        <div class="mb-1.5 flex items-center gap-2">
          <span
            class="inline-block h-2.5 w-2.5 rounded-sm"
            style="background: {hdrColor};"
          ></span>
          <span
            class="text-xs font-semibold uppercase tracking-wide"
            style="color: {hdrColor};"
          >
            {hdrLabel}
          </span>
        </div>
        <!-- Name -->
        <div class="mb-1.5 text-sm font-medium text-slate-100">
          {hoveredNode.label}
        </div>
        <!-- Info table -->
        <table
          class="w-full border-collapse overflow-hidden rounded border border-slate-700 text-xs"
        >
          <tbody>
            {#each infoRows(hoveredNode) as row, i}
              <tr class={i % 2 === 0 ? 'bg-slate-700/40' : 'bg-slate-800'}>
                <td class="whitespace-nowrap px-2 py-1 align-top text-slate-400"
                  >{row.key}</td
                >
                <td class="px-2 py-1 font-mono text-slate-200">{row.value}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </div>
  {/if}
</div>
