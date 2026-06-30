<script lang="ts">
  import { onMount } from 'svelte';
  import {
    listOvsMembers,
    listOvsTable,
    OVS_TABLES,
    type OvsMemberStatus,
  } from '../lib/api';
  import { push } from '../lib/router';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import DataTable from '../components/table/DataTable.svelte';

  // Fleet status: one entry per chassis in the OVS pool. addr is intentionally
  // omitted by the backend, so we only render system-id + connection state.
  let members: OvsMemberStatus[] | null = $state(null);
  let membersLoading = $state(true);
  let membersError = $state('');

  let selectedChassis = $state('');
  let selectedTable = $state('interface');

  let rows: Record<string, unknown>[] = $state([]);
  let rowsLoading = $state(false);
  let rowsError = $state('');

  // Bumping this forces the row effect to refetch even when the selection is
  // unchanged (e.g. the Refresh button).
  let nonce = $state(0);

  const tableOptions = OVS_TABLES.map((t) => ({
    value: t.slug,
    label: t.label,
  }));

  let memberList: OvsMemberStatus[] = $derived(members ?? []);

  let selectedConnected = $derived(
    memberList.find((m) => m.system_id === selectedChassis)?.connected ?? false,
  );

  let summary = $derived.by(() => {
    const connected = memberList.filter((m) => m.connected).length;
    return {
      total: memberList.length,
      connected,
      unreachable: memberList.length - connected,
    };
  });

  // Derive columns from the first 50 rows. OVSDB rows are homogeneous so this
  // captures all columns; _uuid and name lead so the primary view is useful.
  let allColumns = $derived.by(() => {
    if (rows.length === 0) return [];
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local temp, not reactive state
    const keys = new Set<string>();
    for (const row of rows.slice(0, 50)) {
      for (const key of Object.keys(row)) keys.add(key);
    }
    const rest = [...keys].filter((k) => k !== '_uuid' && k !== 'name').sort();
    return ['_uuid', ...(keys.has('name') ? ['name'] : []), ...rest];
  });

  let primaryColumns = $derived(allColumns.slice(0, 6));

  async function loadMembers() {
    membersLoading = true;
    membersError = '';
    try {
      const data = await listOvsMembers();
      members = data;
      // Keep the current selection if it still exists; otherwise default to the
      // first connected chassis (falling back to the first listed).
      const stillThere = data.some((m) => m.system_id === selectedChassis);
      if (!stillThere) {
        const first = data.find((m) => m.connected) ?? data[0];
        selectedChassis = first?.system_id ?? '';
      }
    } catch (e) {
      membersError = e instanceof Error ? e.message : 'Failed to load';
      members = [];
    } finally {
      membersLoading = false;
    }
  }

  async function loadRows(chassis: string, table: string) {
    rowsLoading = true;
    rowsError = '';
    try {
      rows = await listOvsTable(chassis, table);
    } catch (e) {
      rowsError = e instanceof Error ? e.message : 'Failed to load';
      rows = [];
    } finally {
      rowsLoading = false;
    }
  }

  // Refetch rows whenever the selected chassis/table changes, the chassis
  // becomes reachable, or a refresh is requested. Unreachable chassis are not
  // fetched — the backend would return 503.
  $effect(() => {
    const chassis = selectedChassis;
    const table = selectedTable;
    const connected = selectedConnected;
    void nonce;
    if (!chassis || !connected) {
      rows = [];
      rowsError = '';
      rowsLoading = false;
      return;
    }
    loadRows(chassis, table);
  });

  async function refresh() {
    await loadMembers();
    nonce++;
  }

  // Open a row in the per-row detail view, which unpacks its map-typed fields.
  function handleRowClick(row: Record<string, unknown>) {
    const uuid = row._uuid;
    if (typeof uuid !== 'string' || !uuid) return;
    push(
      `/ovs/${encodeURIComponent(selectedChassis)}/${selectedTable}/${encodeURIComponent(uuid)}`,
    );
  }

  onMount(loadMembers);
</script>

<PageHeader
  eyebrow="Monitoring"
  title="OVS Visibility"
  description="Live per-chassis Open vSwitch state, read directly from each node's OVSDB management connection. Pick a chassis and a table to browse its local interfaces, bridges, ports, and controllers."
>
  {#snippet actions()}
    <button class="btn btn-ghost btn-xs border-base-300" onclick={refresh}>
      Refresh
    </button>
  {/snippet}
</PageHeader>

<DataState
  loading={membersLoading}
  error={membersError}
  empty={memberList.length === 0}
  emptyMessage="no OVS chassis configured"
  emptyHint="Set the system-id → mgmt-addr mapping to enable per-chassis OVS visibility."
>
  <StatTiles
    class="mb-4 w-full"
    tiles={[
      { label: 'Chassis', value: summary.total },
      { label: 'Connected', value: summary.connected, variant: 'success' },
      {
        label: 'Unreachable',
        value: summary.unreachable,
        variant: summary.unreachable > 0 ? 'warning' : 'neutral',
      },
    ]}
  />

  <!-- Chassis selector -->
  <div class="mb-4 flex flex-col gap-1.5">
    <span
      class="font-mono text-2xs uppercase tracking-wider text-base-content/45"
      >Chassis</span
    >
    <div class="flex flex-wrap gap-1.5">
      {#each memberList as m (m.system_id)}
        {@const active = m.system_id === selectedChassis}
        <button
          type="button"
          class="flex items-center gap-1.5 rounded border px-2.5 py-1 font-mono text-xs transition-colors {active
            ? 'border-primary bg-primary/10 text-primary'
            : 'border-base-300 text-base-content/70 hover:bg-base-300/50 hover:text-base-content'}"
          aria-pressed={active}
          onclick={() => (selectedChassis = m.system_id)}
        >
          <span
            class={m.connected ? 'text-success' : 'text-error'}
            aria-hidden="true">{m.connected ? '●' : '○'}</span
          >
          <span class="break-all">{m.system_id}</span>
        </button>
      {/each}
    </div>
  </div>

  <!-- Table selector -->
  <div class="mb-4 flex flex-wrap items-center gap-3">
    <span
      class="font-mono text-2xs uppercase tracking-wider text-base-content/45"
      >Table</span
    >
    <SegmentedControl
      options={tableOptions}
      bind:value={selectedTable}
      size="xs"
    />
  </div>

  {#if !selectedConnected}
    <div class="rounded border border-base-300 bg-base-200/30 py-8 text-center">
      <span class="font-mono text-sm text-warning">
        <span class="text-base-content/30">//</span>
        chassis unreachable — the OVSDB management connection to
        <span class="text-base-content/80">{selectedChassis}</span> is down
      </span>
    </div>
  {:else}
    <DataState
      loading={rowsLoading}
      error={rowsError}
      empty={rows.length === 0}
      emptyMessage="no rows"
    >
      <DataTable
        {rows}
        columns={primaryColumns}
        {allColumns}
        onRowClick={handleRowClick}
      />
    </DataState>
  {/if}
</DataState>
