<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '../lib/api';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import DataState from '../components/ui/DataState.svelte';
  import StatTiles from '../components/ui/StatTiles.svelte';
  import SegmentedControl from '../components/ui/SegmentedControl.svelte';
  import Badge from '../components/ui/Badge.svelte';
  import { severityVariant, type Variant } from '../lib/status';

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

  let data = $state<AuditResult | null>(null);
  let loading = $state(true);
  let error = $state('');
  let typeFilter = $state('all');

  const typeOptions = [
    { value: 'all', label: 'All' },
    { value: 'shadow', label: 'Shadow' },
    { value: 'conflict', label: 'Conflict' },
    { value: 'redundant', label: 'Redundant' },
  ];

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

  function typeVariant(t: string): Variant {
    return t === 'shadow' ? 'error' : t === 'conflict' ? 'warning' : 'info';
  }

  onMount(() => load());
</script>

<PageHeader
  eyebrow="Debug"
  title="ACL Security Audit"
  description="Detect shadowed, conflicting, and redundant ACL rules"
/>

<DataState {loading} {error} empty={!data} emptyMessage="no audit data">
  {#if data}
    <StatTiles
      class="mb-4 w-full"
      tiles={[
        { label: 'Total ACLs', value: data.total_acls },
        { label: 'Shadows', value: data.summary.shadows, variant: 'error' },
        {
          label: 'Conflicts',
          value: data.summary.conflicts,
          variant: 'warning',
        },
        {
          label: 'Redundant',
          value: data.summary.redundant,
          variant: 'info',
        },
      ]}
    />

    <div class="mb-4 flex items-center gap-3">
      <SegmentedControl
        options={typeOptions}
        bind:value={typeFilter}
        size="xs"
      />
      <span class="font-mono text-xs text-base-content/55"
        >{filtered.length} findings</span
      >
    </div>

    {#if filtered.length === 0}
      <div class="py-8 text-center">
        <span class="font-mono text-sm text-base-content/40"
          ><span class="text-base-content/30">//</span>
          {data.findings.length === 0
            ? 'no issues found — all ACL rules look clean'
            : 'no findings match the current filter'}</span
        >
      </div>
    {:else}
      <div class="flex flex-col gap-2">
        {#each filtered as finding, i (finding.acl_uuid + '-' + (finding.related_uuid ?? i))}
          <div class="rounded border border-base-300 bg-base-100 p-4">
            <div class="mb-2 flex items-center gap-2">
              <Badge text={finding.type} variant={typeVariant(finding.type)} />
              <Badge
                text={finding.severity}
                variant={severityVariant(finding.severity)}
              />
              {#if finding.context}
                <span class="badge badge-ghost badge-sm">{finding.context}</span
                >
              {/if}
            </div>
            <p class="font-prose mb-2 text-sm">{finding.message}</p>
            <div class="grid grid-cols-2 gap-2 font-mono text-xs">
              <div class="rounded border border-base-300 bg-base-200/60 p-2">
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
                <div class="mt-1 text-base-content/60">
                  {finding.acl_match}
                </div>
              </div>
              {#if finding.related_uuid}
                <div class="rounded border border-base-300 bg-base-200/60 p-2">
                  <div class="font-semibold">
                    Related (priority {finding.related_priority})
                  </div>
                  <div>
                    <span class="badge badge-ghost badge-xs"
                      >{finding.related_action}</span
                    >
                  </div>
                  <div class="mt-1 text-base-content/60">
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
</DataState>
