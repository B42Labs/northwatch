<script lang="ts">
  import { location } from '../../lib/router';
  import { databases, correlatedViews } from '../../lib/tables';
  import { link } from '../../lib/router';

  let collapsedSections: Record<string, boolean> = $state({});

  function toggleSection(key: string) {
    collapsedSections[key] = !collapsedSections[key];
  }

  function isActive(path: string, href: string): boolean {
    return path === href || path.startsWith(href + '/');
  }
</script>

<aside class="min-h-screen w-64 border-r border-base-300 bg-base-100">
  <div class="border-b border-base-300 p-4">
    <a href={link('/')} class="text-xl font-bold text-primary">Northwatch</a>
  </div>

  <ul class="menu menu-sm gap-0.5 p-2">
    <!-- Correlated views -->
    <li>
      <button
        class="menu-title flex justify-between"
        onclick={() => toggleSection('correlated')}
      >
        Correlated
        <span class="text-xs"
          >{collapsedSections['correlated'] ? '+' : '-'}</span
        >
      </button>
      {#if !collapsedSections['correlated']}
        <ul>
          {#each correlatedViews as view}
            <li>
              <a
                href={link(`/correlated/${view.slug}`)}
                class:active={isActive($location, `/correlated/${view.slug}`)}
              >
                {view.label}
              </a>
            </li>
          {/each}
        </ul>
      {/if}
    </li>

    <!-- Visualization -->
    <li>
      <button
        class="menu-title flex justify-between"
        onclick={() => toggleSection('visualization')}
      >
        Visualization
        <span class="text-xs"
          >{collapsedSections['visualization'] ? '+' : '-'}</span
        >
      </button>
      {#if !collapsedSections['visualization']}
        <ul>
          <li>
            <a
              href={link('/topology')}
              class:active={isActive($location, '/topology')}
            >
              Topology
            </a>
          </li>
          <li>
            <a
              href={link('/flows')}
              class:active={isActive($location, '/flows')}
            >
              Flow Pipeline
            </a>
          </li>
          <li>
            <a
              href={link('/security')}
              class:active={isActive($location, '/security')}
            >
              Security Policy
            </a>
          </li>
          <li>
            <a href={link('/nat')} class:active={isActive($location, '/nat')}>
              NAT Overview
            </a>
          </li>
          <li>
            <a href={link('/ha')} class:active={isActive($location, '/ha')}>
              HA Failover
            </a>
          </li>
          <li>
            <a
              href={link('/mac-table')}
              class:active={isActive($location, '/mac-table')}
            >
              MAC Table
            </a>
          </li>
        </ul>
      {/if}
    </li>

    <!-- Debug -->
    <li>
      <button
        class="menu-title flex justify-between"
        onclick={() => toggleSection('debug')}
      >
        Debug
        <span class="text-xs">{collapsedSections['debug'] ? '+' : '-'}</span>
      </button>
      {#if !collapsedSections['debug']}
        <ul>
          <li>
            <a
              href={link('/debug/trace')}
              class:active={isActive($location, '/debug/trace')}
            >
              Packet Trace
            </a>
          </li>
          <li>
            <a
              href={link('/debug/flow-diff')}
              class:active={isActive($location, '/debug/flow-diff')}
            >
              Flow Diff
            </a>
          </li>
          <li>
            <a
              href={link('/debug/connectivity')}
              class:active={isActive($location, '/debug/connectivity')}
            >
              Connectivity
            </a>
          </li>
          <li>
            <a
              href={link('/debug/port-diagnostics')}
              class:active={isActive($location, '/debug/port-diagnostics')}
            >
              Port Diagnostics
            </a>
          </li>
        </ul>
      {/if}
    </li>

    <div class="divider my-1"></div>

    <!-- Database tables -->
    {#each databases as db}
      <li>
        <button
          class="menu-title flex justify-between"
          onclick={() => toggleSection(db.key)}
        >
          {db.label}
          <span class="text-xs">{collapsedSections[db.key] ? '+' : '-'}</span>
        </button>
        {#if !collapsedSections[db.key]}
          <ul>
            {#each db.tables as table}
              <li>
                <a
                  href={link(`/${db.key}/${table.slug}`)}
                  class:active={isActive($location, `/${db.key}/${table.slug}`)}
                >
                  {table.label}
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </li>
    {/each}
  </ul>
</aside>
