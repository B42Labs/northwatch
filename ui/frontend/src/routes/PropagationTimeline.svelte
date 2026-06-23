<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as d3 from 'd3';
  import {
    getPropagationTimeline,
    getPropagationHeatmap,
    type PropagationEvent,
    type ChassisPropSummary,
  } from '../lib/api';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Card from '../components/ui/Card.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';

  type TimeRange = { label: string; ms: number };
  const TIME_RANGES: TimeRange[] = [
    { label: '15m', ms: 15 * 60 * 1000 },
    { label: '1h', ms: 60 * 60 * 1000 },
    { label: '6h', ms: 6 * 60 * 60 * 1000 },
    { label: '24h', ms: 24 * 60 * 60 * 1000 },
  ];

  let events: PropagationEvent[] = $state([]);
  let summaries: ChassisPropSummary[] = $state([]);
  let currentGen = $state(0);
  let loading = $state(true);
  let error = $state('');
  let selectedRange = $state(TIME_RANGES[1]);
  let chassisFilter = $state('');

  let svgRef: SVGSVGElement | undefined = $state();
  let refreshTimer: ReturnType<typeof setInterval> | undefined;

  const chassisNames: string[] = $derived(
    [...new Set(summaries.map((s) => s.chassis))].sort(),
  );

  async function load() {
    error = '';
    const since = Date.now() - selectedRange.ms;
    try {
      const [timeline, heatmap] = await Promise.all([
        getPropagationTimeline({
          since,
          chassis: chassisFilter || undefined,
          limit: 5000,
        }),
        getPropagationHeatmap({ since }),
      ]);
      events = timeline.events ?? [];
      currentGen = timeline.current_generation;
      summaries = heatmap.chassis ?? [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

  function renderChart() {
    if (!svgRef || events.length === 0) return;

    const svg = d3.select(svgRef);
    svg.selectAll('*').remove();

    const rect = svgRef.getBoundingClientRect();
    const width = rect.width;
    const height = 320;
    const margin = { top: 20, right: 20, bottom: 40, left: 60 };
    const innerW = width - margin.left - margin.right;
    const innerH = height - margin.top - margin.bottom;

    svg.attr('viewBox', `0 0 ${width} ${height}`);

    const g = svg
      .append('g')
      .attr('transform', `translate(${margin.left},${margin.top})`);

    // Latency-based color scale (green → yellow → red)
    const yMax = d3.max(events, (e) => e.latency_ms) ?? 1000;
    const latencyColor = d3
      .scaleLinear<string>()
      .domain([0, yMax * 0.5, yMax])
      .range(['#22c55e', '#eab308', '#ef4444'])
      .clamp(true);

    const xExtent = d3.extent(events, (e) => e.chassis_timestamp_ms) as [
      number,
      number,
    ];
    const x = d3.scaleTime().domain(xExtent).range([0, innerW]).nice();

    const y = d3
      .scaleLinear()
      .domain([0, yMax * 1.1])
      .range([innerH, 0])
      .nice();

    // Deterministic jitter per chassis so overlapping points spread out
    function jitter(chassis: string): number {
      let h = 0;
      for (let i = 0; i < chassis.length; i++) {
        h = (h * 31 + chassis.charCodeAt(i)) | 0;
      }
      return ((h & 0xffff) / 0x8000 - 1) * innerW * 0.012;
    }

    // Smart time format: include seconds when range is narrow
    const xRangeMs = xExtent[1] - xExtent[0];
    const timeFmt =
      xRangeMs < 5 * 60 * 1000
        ? d3.timeFormat('%H:%M:%S')
        : d3.timeFormat('%H:%M');

    // Axes
    g.append('g')
      .attr('transform', `translate(0,${innerH})`)
      .call(
        d3
          .axisBottom(x)
          .ticks(6)
          .tickFormat((d) => timeFmt(d as Date)),
      )
      .selectAll('text')
      .attr('class', 'fill-base-content/60 text-xs');

    g.append('g')
      .call(
        d3
          .axisLeft(y)
          .ticks(6)
          .tickFormat((d) => formatMs(d as number)),
      )
      .selectAll('text')
      .attr('class', 'fill-base-content/60 text-xs');

    // Axis labels
    g.append('text')
      .attr('x', innerW / 2)
      .attr('y', innerH + 34)
      .attr('text-anchor', 'middle')
      .attr('class', 'fill-base-content/40 text-xs')
      .text('Time');

    g.append('text')
      .attr('x', -innerH / 2)
      .attr('y', -48)
      .attr('text-anchor', 'middle')
      .attr('transform', 'rotate(-90)')
      .attr('class', 'fill-base-content/40 text-xs')
      .text('Latency');

    // Grid lines
    g.append('g')
      .attr('class', 'opacity-10')
      .call(
        d3
          .axisLeft(y)
          .ticks(6)
          .tickSize(-innerW)
          .tickFormat(() => ''),
      )
      .select('.domain')
      .remove();

    // Tooltip div
    const tooltip = d3
      .select(svgRef.parentNode as Element)
      .selectAll('.chart-tooltip')
      .data([null])
      .join('div')
      .attr(
        'class',
        'chart-tooltip absolute pointer-events-none bg-base-200 text-xs p-2 rounded shadow-lg border border-base-300 opacity-0 z-50',
      );

    // Point cloud: small semi-transparent dots with jitter
    g.selectAll('circle')
      .data(events)
      .join('circle')
      .attr('cx', (d) => x(d.chassis_timestamp_ms) + jitter(d.chassis))
      .attr('cy', (d) => y(d.latency_ms))
      .attr('r', 2.5)
      .attr('fill', (d) => latencyColor(d.latency_ms))
      .attr('opacity', 0.5)
      .attr('stroke', 'none')
      .on('mouseenter', function (event, d) {
        d3.select(this)
          .attr('r', 5)
          .attr('opacity', 1)
          .attr('stroke', '#000')
          .attr('stroke-width', 1);
        tooltip
          .style('opacity', '1')
          .html(
            `<strong>${d.chassis}</strong><br/>` +
              `Gen: ${d.generation}<br/>` +
              `Latency: ${formatMs(d.latency_ms)}<br/>` +
              `Time: ${new Date(d.chassis_timestamp_ms).toLocaleTimeString()}`,
          );
      })
      .on('mousemove', function (event) {
        const [mx, my] = d3.pointer(event, svgRef!.parentNode as Element);
        tooltip.style('left', mx + 12 + 'px').style('top', my - 10 + 'px');
      })
      .on('mouseleave', function () {
        d3.select(this)
          .attr('r', 2.5)
          .attr('opacity', 0.5)
          .attr('stroke', 'none');
        tooltip.style('opacity', '0');
      });
  }

  function latencyClass(ms: number): string {
    if (ms < 1000) return 'bg-success/20 text-success';
    if (ms < 5000) return 'bg-warning/20 text-warning';
    if (ms < 15000) return 'bg-orange-500/20 text-orange-600';
    return 'bg-error/20 text-error';
  }

  function formatMs(ms: number): string {
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  }

  onMount(async () => {
    await load();
    refreshTimer = setInterval(load, 10_000);
  });

  onDestroy(() => {
    if (refreshTimer) clearInterval(refreshTimer);
  });

  $effect(() => {
    // Re-render chart when events or svgRef change
    if (svgRef && events) renderChart();
  });
</script>

<PageHeader
  eyebrow="Monitoring"
  title="Config Propagation Timeline"
  description="Per-chassis nb_cfg propagation latency over time."
>
  {#snippet meta()}
    {#if currentGen > 0}
      <Badge text="gen {currentGen}" variant="neutral" />
    {/if}
  {/snippet}
  {#snippet actions()}
    {#if chassisNames.length > 0}
      <select
        class="select select-bordered select-sm bg-base-200/60 font-mono"
        bind:value={chassisFilter}
        onchange={load}
        aria-label="Filter by chassis"
      >
        <option value="">All chassis</option>
        {#each chassisNames as name (name)}
          <option value={name}>{name}</option>
        {/each}
      </select>
    {/if}
    <SegmentedControl
      options={TIME_RANGES.map((r) => ({ value: r.label, label: r.label }))}
      value={selectedRange.label}
      onchange={(label) => {
        const range = TIME_RANGES.find((r) => r.label === label);
        if (range) {
          selectedRange = range;
          load();
        }
      }}
    />
  {/snippet}
</PageHeader>

<DataState {loading} {error}>
  <!-- Timeline Chart -->
  <Card title="Propagation Timeline" class="mb-6">
    {#if events.length === 0}
      <div class="py-8 text-center font-mono text-sm text-base-content/40">
        <span class="text-base-content/30">//</span> no propagation events recorded
        in this time range
      </div>
    {:else}
      <div class="relative w-full">
        <svg bind:this={svgRef} class="w-full" style="height: 320px"></svg>
      </div>
    {/if}
  </Card>

  <!-- Heatmap Summary Table -->
  <Card title="Chassis Propagation Summary" padding="none">
    {#if summaries.length === 0}
      <div class="py-8 text-center font-mono text-sm text-base-content/40">
        <span class="text-base-content/30">//</span> no chassis data available
      </div>
    {:else}
      <div class="overflow-x-auto rounded border border-base-300">
        <table class="table table-xs font-mono">
          <thead>
            <tr>
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >Chassis</th
              >
              <th
                class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                >Hostname</th
              >
              <th
                class="bg-base-200 text-right text-2xs uppercase tracking-wider text-base-content/55"
                >Count</th
              >
              <th
                class="bg-base-200 text-right text-2xs uppercase tracking-wider text-base-content/55"
                >Avg</th
              >
              <th
                class="bg-base-200 text-right text-2xs uppercase tracking-wider text-base-content/55"
                >P50</th
              >
              <th
                class="bg-base-200 text-right text-2xs uppercase tracking-wider text-base-content/55"
                >P95</th
              >
              <th
                class="bg-base-200 text-right text-2xs uppercase tracking-wider text-base-content/55"
                >P99</th
              >
              <th
                class="bg-base-200 text-right text-2xs uppercase tracking-wider text-base-content/55"
                >Max</th
              >
            </tr>
          </thead>
          <tbody>
            {#each summaries as s (s.chassis)}
              <tr class="border-base-300/60">
                <td class="text-xs">{s.chassis}</td>
                <td class="text-xs text-base-content/60">{s.hostname || '-'}</td>
                <td class="text-right text-xs">{s.count}</td>
                <td class="text-right">
                  <span
                    class="rounded px-1.5 py-0.5 text-xs font-medium {latencyClass(
                      s.avg_ms,
                    )}"
                  >
                    {formatMs(s.avg_ms)}
                  </span>
                </td>
                <td class="text-right">
                  <span
                    class="rounded px-1.5 py-0.5 text-xs font-medium {latencyClass(
                      s.p50_ms,
                    )}"
                  >
                    {formatMs(s.p50_ms)}
                  </span>
                </td>
                <td class="text-right">
                  <span
                    class="rounded px-1.5 py-0.5 text-xs font-medium {latencyClass(
                      s.p95_ms,
                    )}"
                  >
                    {formatMs(s.p95_ms)}
                  </span>
                </td>
                <td class="text-right">
                  <span
                    class="rounded px-1.5 py-0.5 text-xs font-medium {latencyClass(
                      s.p99_ms,
                    )}"
                  >
                    {formatMs(s.p99_ms)}
                  </span>
                </td>
                <td class="text-right">
                  <span
                    class="rounded px-1.5 py-0.5 text-xs font-medium {latencyClass(
                      s.max_ms,
                    )}"
                  >
                    {formatMs(s.max_ms)}
                  </span>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </Card>
</DataState>
