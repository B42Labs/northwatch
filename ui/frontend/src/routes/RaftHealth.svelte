<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';

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

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">Raft Cluster Health</h1>
    <p class="text-sm text-base-content/60">
      OVSDB Raft cluster connection status for NB and SB databases
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if loading}
    <LoadingSpinner />
  {:else if data}
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      {#each [{ label: 'Northbound', db: data.nb }, { label: 'Southbound', db: data.sb }] as { label, db } (label)}
        <div class="card border border-base-300 bg-base-100 shadow-sm">
          <div class="card-body p-4">
            <h2 class="card-title text-base">
              {label}
              <span
                class="badge badge-sm {db.connected
                  ? 'badge-success'
                  : 'badge-error'}"
              >
                {db.connected ? 'Connected' : 'Disconnected'}
              </span>
            </h2>
            <div class="text-sm text-base-content/60">
              {db.endpoints} endpoint(s)
            </div>

            {#if db.connections.length > 0}
              <div class="mt-2 overflow-x-auto">
                <table class="table table-xs">
                  <thead>
                    <tr>
                      <th>Target</th>
                      <th>Status</th>
                      <th>Connected</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each db.connections as conn (conn.target)}
                      <tr>
                        <td class="font-mono text-xs">{conn.target}</td>
                        <td class="text-xs">{conn.status ?? '-'}</td>
                        <td>
                          <span
                            class="badge badge-xs {conn.is_connected
                              ? 'badge-success'
                              : 'badge-error'}"
                          >
                            {conn.is_connected ? 'yes' : 'no'}
                          </span>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            {:else}
              <div class="text-sm text-base-content/40">
                No connection entries
              </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
