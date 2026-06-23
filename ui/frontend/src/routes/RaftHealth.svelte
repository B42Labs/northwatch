<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import Card from '../components/ui/Card.svelte';
  import Badge from '../components/ui/Badge.svelte';

  interface ConnectionDetail {
    uuid: string;
    target: string;
    is_connected: boolean;
    status?: string;
  }
  interface RaftDBHealth {
    connected: boolean;
    endpoints: number;
    active_endpoint?: string;
    connections: ConnectionDetail[];
  }
  interface RaftHealthResult {
    nb: RaftDBHealth;
    sb: RaftDBHealth;
  }

  let data: RaftHealthResult | null = $state(null);
  let loading = $state(true);
  let error = $state('');

  async function load() {
    loading = true;
    error = '';
    try {
      data = await get<RaftHealthResult>('/api/v1/telemetry/raft-health');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

  onMount(() => load());
</script>

<PageHeader
  eyebrow="Monitoring"
  title="Raft Cluster Health"
  description="OVSDB Raft cluster connection status for the Northbound and Southbound databases."
/>

<DataState {loading} {error} empty={!data} emptyMessage="no health data">
  {#if data}
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      {#each [{ label: 'Northbound', db: data.nb }, { label: 'Southbound', db: data.sb }] as { label, db } (label)}
        <Card title={label} accent={db.connected ? 'success' : 'error'}>
          {#snippet actions()}
            <Badge
              text={db.connected ? 'connected' : 'disconnected'}
              variant={db.connected ? 'success' : 'error'}
              glyph={db.connected ? '●' : '○'}
            />
          {/snippet}

          <div class="mb-2 font-mono text-xs text-base-content/55">
            {db.endpoints} endpoint{db.endpoints === 1 ? '' : 's'}
            {#if db.active_endpoint}
              · active <span class="text-base-content/80"
                >{db.active_endpoint}</span
              >
            {/if}
          </div>

          {#if db.connections.length > 0}
            <div class="overflow-x-auto rounded border border-base-300">
              <table class="table table-xs font-mono">
                <thead>
                  <tr>
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >Target</th
                    >
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >Status</th
                    >
                    <th
                      class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                      >Up</th
                    >
                  </tr>
                </thead>
                <tbody>
                  {#each db.connections as conn (conn.target)}
                    <tr class="border-base-300/60">
                      <td class="text-xs">{conn.target}</td>
                      <td class="text-xs text-base-content/70"
                        >{conn.status ?? '-'}</td
                      >
                      <td>
                        <Badge
                          text={conn.is_connected ? 'yes' : 'no'}
                          variant={conn.is_connected ? 'success' : 'error'}
                        />
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else}
            <div class="font-mono text-xs text-base-content/40">
              <span class="text-base-content/30">//</span> no connection entries
            </div>
          {/if}
        </Card>
      {/each}
    </div>
  {/if}
</DataState>
