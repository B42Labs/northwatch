<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';

  interface AuditFinding {
    type: string;
    severity: string;
    message: string;
    acl_uuid: string;
    acl_priority: number;
    acl_match: string;
    acl_action: string;
    acl_direction: string;
    related_uuid?: string;
    related_priority?: number;
    related_match?: string;
    related_action?: string;
    context?: string;
  }

  interface AuditResult {
    total_acls: number;
    findings: AuditFinding[];
    summary: { shadows: number; conflicts: number; redundant: number };
  }

  let data: AuditResult | null = $state(null);
  let loading = $state(true);
  let error = $state('');
  let typeFilter = $state<'all' | 'shadow' | 'conflict' | 'redundant'>('all');

  let filtered = $derived(
    (data?.findings ?? []).filter(
      (f) => typeFilter === 'all' || f.type === typeFilter,
    ),
  );

  async function load() {
    loading = true;
    error = '';
    try {
      data = await get<AuditResult>('/api/v1/debug/acl-audit');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load ACL audit';
    } finally {
      loading = false;
    }
  }

  function severityBadge(s: string): string {
    return s === 'error'
      ? 'badge-error'
      : s === 'warning'
        ? 'badge-warning'
        : 'badge-info';
  }

  function typeBadge(t: string): string {
    return t === 'shadow'
      ? 'badge-error'
      : t === 'conflict'
        ? 'badge-warning'
        : 'badge-info';
  }

  onMount(() => load());
</script>

<div>
  <div class="mb-4">
    <h1 class="text-xl font-bold">ACL Security Audit</h1>
    <p class="text-sm text-base-content/60">
      Detect shadowed, conflicting, and redundant ACL rules
    </p>
  </div>

  {#if error}
    <ErrorAlert message={error} />
  {:else if loading}
    <LoadingSpinner />
  {:else if data}
    <div class="stats mb-4 w-full border border-base-300 bg-base-100 shadow-sm">
      <div class="stat">
        <div class="stat-title">Total ACLs</div>
        <div class="stat-value text-lg">{data.total_acls}</div>
      </div>
      <div class="stat">
        <div class="stat-title">Shadows</div>
        <div class="stat-value text-lg text-error">{data.summary.shadows}</div>
      </div>
      <div class="stat">
        <div class="stat-title">Conflicts</div>
        <div class="stat-value text-lg text-warning">
          {data.summary.conflicts}
        </div>
      </div>
      <div class="stat">
        <div class="stat-title">Redundant</div>
        <div class="stat-value text-lg text-info">{data.summary.redundant}</div>
      </div>
    </div>

    <div class="mb-4 flex items-center gap-3">
      <div class="join">
        {#each ['all', 'shadow', 'conflict', 'redundant'] as t (t)}
          <button
            class="btn join-item btn-xs {typeFilter === t ? 'btn-active' : ''}"
            onclick={() => (typeFilter = t as typeof typeFilter)}
            >{t === 'all'
              ? 'All'
              : t.charAt(0).toUpperCase() + t.slice(1)}</button
          >
        {/each}
      </div>
      <span class="text-sm text-base-content/50"
        >{filtered.length} findings</span
      >
    </div>

    {#if filtered.length === 0}
      <div class="py-8 text-center text-sm text-base-content/40">
        {data.findings.length === 0
          ? 'No issues found - all ACL rules look clean'
          : 'No findings match the current filter'}
      </div>
    {:else}
      <div class="flex flex-col gap-2">
        {#each filtered as finding, i (finding.acl_uuid + '-' + (finding.related_uuid ?? i))}
          <div
            class="rounded-lg border border-base-300 bg-base-100 p-4 shadow-sm"
          >
            <div class="mb-2 flex items-center gap-2">
              <span class="badge badge-sm {typeBadge(finding.type)}"
                >{finding.type}</span
              >
              <span class="badge badge-sm {severityBadge(finding.severity)}"
                >{finding.severity}</span
              >
              {#if finding.context}
                <span class="badge badge-ghost badge-sm">{finding.context}</span
                >
              {/if}
            </div>
            <p class="mb-2 text-sm">{finding.message}</p>
            <div class="grid grid-cols-2 gap-2 text-xs">
              <div class="rounded bg-base-200 p-2">
                <div class="font-semibold">
                  ACL (priority {finding.acl_priority})
                </div>
                <div>
                  <span class="badge badge-ghost badge-xs"
                    >{finding.acl_action}</span
                  >
                  <span class="badge badge-ghost badge-xs"
                    >{finding.acl_direction}</span
                  >
                </div>
                <div class="mt-1 font-mono text-base-content/60">
                  {finding.acl_match}
                </div>
              </div>
              {#if finding.related_uuid}
                <div class="rounded bg-base-200 p-2">
                  <div class="font-semibold">
                    Related (priority {finding.related_priority})
                  </div>
                  <div>
                    <span class="badge badge-ghost badge-xs"
                      >{finding.related_action}</span
                    >
                  </div>
                  <div class="mt-1 font-mono text-base-content/60">
                    {finding.related_match}
                  </div>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
