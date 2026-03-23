<script lang="ts">
  import { link } from '../lib/router';
  import { capabilities, writeEnabled } from '../lib/capabilitiesStore';
  import Badge from '../components/ui/Badge.svelte';
</script>

<div class="mx-auto max-w-4xl">
  <h1 class="mb-2 text-3xl font-bold">Northwatch</h1>
  <p class="mb-6 text-base-content/70">OVN Database Browser & Analyzer</p>

  {#if $capabilities.length > 0}
    <div class="card mb-6 bg-base-100 shadow-sm">
      <div class="card-body">
        <h2 class="card-title text-sm">Active Capabilities</h2>
        <div class="flex flex-wrap gap-2">
          {#each $capabilities as cap (cap)}
            <Badge text={cap} variant="primary" />
          {/each}
        </div>
      </div>
    </div>
  {/if}

  <div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
    <!-- Correlated -->
    <div class="card bg-base-100 shadow-sm">
      <div class="card-body">
        <h2 class="card-title text-base">Correlated</h2>
        <p class="mb-2 text-xs text-base-content/60">
          Enriched NB/SB views with cross-database correlation
        </p>
        <ul class="flex flex-col gap-1">
          <li>
            <a
              href={link('/correlated/logical-switches')}
              class="link-hover link text-sm">Logical Switches</a
            >
          </li>
          <li>
            <a
              href={link('/correlated/logical-routers')}
              class="link-hover link text-sm">Logical Routers</a
            >
          </li>
          <li>
            <a
              href={link('/correlated/chassis')}
              class="link-hover link text-sm">Chassis</a
            >
          </li>
        </ul>
      </div>
    </div>

    <!-- Visualization -->
    <div class="card bg-base-100 shadow-sm">
      <div class="card-body">
        <h2 class="card-title text-base">Visualization</h2>
        <p class="mb-2 text-xs text-base-content/60">
          Topology, flows, and network policy views
        </p>
        <ul class="flex flex-col gap-1">
          <li>
            <a href={link('/topology')} class="link-hover link text-sm"
              >Topology</a
            >
          </li>
          <li>
            <a href={link('/flows')} class="link-hover link text-sm"
              >Flow Pipeline</a
            >
          </li>
          <li>
            <a href={link('/security')} class="link-hover link text-sm"
              >Security Policy</a
            >
          </li>
          <li>
            <a href={link('/nat')} class="link-hover link text-sm"
              >NAT Overview</a
            >
          </li>
          <li>
            <a href={link('/ha')} class="link-hover link text-sm">HA Failover</a
            >
          </li>
          <li>
            <a href={link('/mac-table')} class="link-hover link text-sm"
              >MAC Table</a
            >
          </li>
          <li>
            <a href={link('/load-balancers')} class="link-hover link text-sm"
              >Load Balancers</a
            >
          </li>
        </ul>
      </div>
    </div>

    <!-- Debug -->
    <div class="card bg-base-100 shadow-sm">
      <div class="card-body">
        <h2 class="card-title text-base">Debug</h2>
        <p class="mb-2 text-xs text-base-content/60">
          Packet tracing, diagnostics, and audit tools
        </p>
        <ul class="flex flex-col gap-1">
          <li>
            <a href={link('/debug/trace')} class="link-hover link text-sm"
              >Packet Trace</a
            >
          </li>
          <li>
            <a href={link('/debug/flow-diff')} class="link-hover link text-sm"
              >Flow Diff</a
            >
          </li>
          <li>
            <a
              href={link('/debug/connectivity')}
              class="link-hover link text-sm">Connectivity</a
            >
          </li>
          <li>
            <a
              href={link('/debug/port-diagnostics')}
              class="link-hover link text-sm">Port Diagnostics</a
            >
          </li>
          <li>
            <a href={link('/debug/acl-audit')} class="link-hover link text-sm"
              >ACL Audit</a
            >
          </li>
          <li>
            <a
              href={link('/debug/stale-entries')}
              class="link-hover link text-sm">Stale Entries</a
            >
          </li>
        </ul>
      </div>
    </div>

    <!-- History & Events -->
    <div class="card bg-base-100 shadow-sm">
      <div class="card-body">
        <h2 class="card-title text-base">History & Events</h2>
        <p class="mb-2 text-xs text-base-content/60">
          Database change events and state snapshots
        </p>
        <ul class="flex flex-col gap-1">
          <li>
            <a href={link('/events')} class="link-hover link text-sm">Events</a>
          </li>
          <li>
            <a href={link('/history')} class="link-hover link text-sm"
              >Snapshots</a
            >
          </li>
        </ul>
      </div>
    </div>

    <!-- Monitoring -->
    <div class="card bg-base-100 shadow-sm">
      <div class="card-body">
        <h2 class="card-title text-base">Monitoring</h2>
        <p class="mb-2 text-xs text-base-content/60">
          Cluster health and RAFT consensus status
        </p>
        <ul class="flex flex-col gap-1">
          <li>
            <a href={link('/raft-health')} class="link-hover link text-sm"
              >Raft Health</a
            >
          </li>
        </ul>
      </div>
    </div>

    <!-- Write (conditional) -->
    {#if $writeEnabled}
      <div class="card bg-base-100 shadow-sm">
        <div class="card-body">
          <h2 class="card-title text-base">Write</h2>
          <p class="mb-2 text-xs text-base-content/60">
            Create, update, or delete OVN entities
          </p>
          <ul class="flex flex-col gap-1">
            <li>
              <a href={link('/write')} class="link-hover link text-sm"
                >Operation Builder</a
              >
            </li>
            <li>
              <a href={link('/write/audit')} class="link-hover link text-sm"
                >Audit Log</a
              >
            </li>
          </ul>
        </div>
      </div>
    {/if}
  </div>
</div>
