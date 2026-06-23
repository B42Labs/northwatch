<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api';
  import type { Variant } from '../lib/status';
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
  interface RaftMember {
    endpoint: string;
    reachable: boolean;
    server_id?: string;
    leader: boolean;
    connected: boolean;
    index?: number;
  }
  interface RaftCluster {
    available: boolean;
    model?: string; // standalone | clustered | relay
    cluster_id?: string;
    split_brain?: boolean;
    total: number;
    reachable: number;
    has_leader: boolean;
    leader_id?: string;
    members: RaftMember[];
  }
  interface RaftDBHealth {
    client_connected: boolean;
    cluster: RaftCluster;
    listeners: ConnectionDetail[];
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

  // Card accent: red if Northwatch can't reach the DB or the cluster has no/split
  // leader; amber if a configured member is unreachable; green otherwise.
  function accentFor(db: RaftDBHealth): Variant {
    if (!db.client_connected) return 'error';
    const c = db.cluster;
    if (c.available && c.model === 'clustered') {
      if (c.split_brain || !c.has_leader) return 'error';
      if (c.reachable < c.total) return 'warning';
      return 'success';
    }
    return 'success';
  }

  function modelVariant(model: string | undefined): Variant {
    return model === 'clustered' ? 'info' : 'neutral';
  }

  function shortId(id: string | undefined): string {
    return id ? id.slice(0, 8) : '—';
  }
</script>

<PageHeader
  eyebrow="Monitoring"
  title="Raft Cluster Health"
  description="Real OVSDB Raft cluster membership (aggregated from each endpoint's _Server database) plus Northwatch's own connection to the Northbound and Southbound databases."
/>

<DataState {loading} {error} empty={!data} emptyMessage="no health data">
  {#if data}
    <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
      {#each [{ label: 'Northbound', db: data.nb }, { label: 'Southbound', db: data.sb }] as { label, db } (label)}
        <Card title={label} accent={accentFor(db)}>
          {#snippet actions()}
            <Badge
              text={db.client_connected
                ? 'client connected'
                : 'client disconnected'}
              variant={db.client_connected ? 'success' : 'error'}
              glyph={db.client_connected ? '●' : '○'}
            />
          {/snippet}

          <!-- Raft cluster membership aggregated from per-endpoint _Server rows -->
          <div
            class="mb-1.5 text-2xs uppercase tracking-wider text-base-content/45"
          >
            Cluster
          </div>
          {#if db.cluster.available}
            <div
              class="mb-2 flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-xs"
            >
              <Badge
                text={db.cluster.model ?? 'unknown'}
                variant={modelVariant(db.cluster.model)}
              />
              <span class="text-base-content/55"
                >{db.cluster.reachable}/{db.cluster.total} reachable</span
              >
              <span class="text-base-content/30">·</span>
              {#if db.cluster.has_leader}
                <span class="text-base-content/55">leader</span>
                <span class="text-base-content/80" title={db.cluster.leader_id}
                  >{shortId(db.cluster.leader_id)}</span
                >
              {:else}
                <Badge text="no leader" variant="error" />
              {/if}
              {#if db.cluster.split_brain}
                <Badge text="split-brain: cid mismatch" variant="error" />
              {:else if db.cluster.cluster_id}
                <span class="text-base-content/30">·</span>
                <span class="text-base-content/55">cid</span>
                <span class="text-base-content/80" title={db.cluster.cluster_id}
                  >{shortId(db.cluster.cluster_id)}</span
                >
              {/if}
            </div>

            {#if db.cluster.model === 'clustered'}
              <div class="overflow-x-auto rounded border border-base-300">
                <table class="table table-xs font-mono">
                  <thead>
                    <tr>
                      {#each ['Endpoint', 'Server', 'Role', 'Contact', 'Index'] as h (h)}
                        <th
                          class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                          >{h}</th
                        >
                      {/each}
                    </tr>
                  </thead>
                  <tbody>
                    {#each db.cluster.members as m (m.endpoint)}
                      <tr
                        class="border-base-300/60"
                        class:opacity-50={!m.reachable}
                      >
                        <td class="text-xs">{m.endpoint}</td>
                        <td
                          class="text-xs text-base-content/80"
                          title={m.server_id}>{shortId(m.server_id)}</td
                        >
                        <td>
                          {#if !m.reachable}
                            <span class="text-base-content/40">—</span>
                          {:else}
                            <Badge
                              text={m.leader ? 'leader' : 'follower'}
                              variant={m.leader ? 'info' : 'neutral'}
                            />
                          {/if}
                        </td>
                        <td>
                          {#if !m.reachable}
                            <Badge
                              text="unreachable"
                              variant="error"
                              glyph="○"
                            />
                          {:else}
                            <Badge
                              text={m.connected ? 'in contact' : 'lost'}
                              variant={m.connected ? 'success' : 'warning'}
                              glyph={m.connected ? '●' : '○'}
                            />
                          {/if}
                        </td>
                        <td class="text-xs text-base-content/70"
                          >{m.reachable ? (m.index ?? '—') : '—'}</td
                        >
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
              <div class="mt-1.5 font-mono text-2xs text-base-content/40">
                <span class="text-base-content/30">//</span> members are the endpoints
                Northwatch is configured with; a member down at startup stays unreachable
                until restart
              </div>
            {:else}
              <div class="font-mono text-xs text-base-content/50">
                <span class="text-base-content/30">//</span> not clustered —
                Raft does not apply to a {db.cluster.model} database
              </div>
            {/if}
          {:else}
            <div class="mb-3 font-mono text-xs text-base-content/50">
              <span class="text-base-content/30">//</span> cluster state
              unavailable — no endpoint exposed
              <span class="text-base-content/70">_Server</span>
              (standalone database, offline snapshot, or members down)
            </div>
          {/if}

          <!-- Configured connection listeners (OVSDB Connection table) -->
          <div
            class="mb-1.5 mt-3 text-2xs uppercase tracking-wider text-base-content/45"
          >
            Listeners
          </div>
          {#if db.listeners.length > 0}
            <div class="overflow-x-auto rounded border border-base-300">
              <table class="table table-xs font-mono">
                <thead>
                  <tr>
                    {#each ['Target', 'Status', 'is_connected'] as h (h)}
                      <th
                        class="bg-base-200 text-2xs uppercase tracking-wider text-base-content/55"
                        >{h}</th
                      >
                    {/each}
                  </tr>
                </thead>
                <tbody>
                  {#each db.listeners as conn (conn.uuid)}
                    <tr class="border-base-300/60">
                      <td class="text-xs">{conn.target}</td>
                      <td class="text-xs text-base-content/70"
                        >{conn.status || '—'}</td
                      >
                      <td>
                        <Badge
                          text={conn.is_connected ? 'yes' : 'no'}
                          variant={conn.is_connected ? 'success' : 'neutral'}
                        />
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
            <div class="mt-1.5 font-mono text-2xs text-base-content/40">
              <span class="text-base-content/30">//</span> passive listeners (ptcp:/pssl:)
              normally report is_connected=no; liveness is in status (bound_port,
              n_connections)
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
