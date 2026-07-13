<script lang="ts">
  import {
    previewOperations,
    applyPlan,
    cancelPlan,
    type WriteOperation,
    type Plan,
  } from '../lib/writeApi';
  import { link } from '../lib/router';
  import PageContainer from '../components/ui/PageContainer.svelte';
  import PageHeader from '../components/ui/PageHeader.svelte';
  import FormField from '../components/ui/FormField.svelte';
  import OperationForm from '../components/write/OperationForm.svelte';
  import OperationList from '../components/write/OperationList.svelte';
  import PlanReview from '../components/write/PlanReview.svelte';
  import ErrorAlert from '../components/ui/ErrorAlert.svelte';
  import LoadingSpinner from '../components/ui/LoadingSpinner.svelte';

  let { query = {} }: { query?: Record<string, string> } = $props();

  let step: 'build' | 'review' | 'result' = $state('build');
  let operations: WriteOperation[] = $state([]);
  let reason = $state('');
  let plan: Plan | null = $state(null);
  let error = $state('');
  let loading = $state(false);
  let applying = $state(false);
  let applyResult: {
    success: boolean;
    auditId?: number;
    error?: string;
  } | null = $state(null);

  // Pre-populate from query params (deep-links from RawDetail)
  let initialAction = $derived(query.action || '');
  let initialTable = $derived(query.table || '');
  let initialUuid = $derived(query.uuid || '');

  function addOperation(op: WriteOperation) {
    operations = [...operations, op];
  }

  function removeOperation(index: number) {
    operations = operations.filter((_, i) => i !== index);
  }

  async function handlePreview() {
    if (operations.length === 0) {
      error = 'Add at least one operation';
      return;
    }
    error = '';
    loading = true;
    try {
      plan = await previewOperations(operations, reason || undefined);
      step = 'review';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Preview failed';
    } finally {
      loading = false;
    }
  }

  async function handleApply() {
    if (!plan) return;
    applying = true;
    error = '';
    try {
      const entry = await applyPlan(plan.id, plan.apply_token);
      applyResult = { success: true, auditId: entry.id };
      step = 'result';
    } catch (e) {
      applyResult = {
        success: false,
        error: e instanceof Error ? e.message : 'Apply failed',
      };
      step = 'result';
    } finally {
      applying = false;
    }
  }

  async function handleCancel() {
    if (plan) {
      try {
        await cancelPlan(plan.id);
      } catch {
        // Plan may have already expired
      }
    }
    plan = null;
    step = 'build';
  }

  function handleReset() {
    operations = [];
    reason = '';
    plan = null;
    error = '';
    applyResult = null;
    step = 'build';
  }

  let stepLabels = ['Build', 'Review', 'Result'];
  let stepIndex = $derived(step === 'build' ? 0 : step === 'review' ? 1 : 2);
</script>

<PageContainer width="prose">
  <PageHeader
    eyebrow="Write"
    title="Write Operations"
    description="Build, preview, and apply changes to OVN Northbound tables."
  />

  <!-- Stepper -->
  <ul class="steps steps-horizontal mb-6 w-full">
    {#each stepLabels as label, i (i)}
      <li
        class="step font-mono text-2xs tracking-wider uppercase"
        class:step-primary={i <= stepIndex}
      >
        {label}
      </li>
    {/each}
  </ul>

  {#if error}
    <div class="mb-4">
      <ErrorAlert message={error} />
    </div>
  {/if}

  <!-- Step 1: Build -->
  {#if step === 'build'}
    <div class="flex flex-col gap-4">
      <OperationForm
        onAdd={addOperation}
        {initialAction}
        {initialTable}
        {initialUuid}
      />

      <div>
        <h2
          class="mb-2 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
        >
          Batch ({operations.length} operation{operations.length !== 1
            ? 's'
            : ''})
        </h2>
        <OperationList {operations} onRemove={removeOperation} />
      </div>

      {#if operations.length > 0}
        <div class="flex items-end gap-3">
          <div class="flex-1">
            <FormField label="Batch Reason (optional)" forId="batch-reason">
              <input
                id="batch-reason"
                type="text"
                class="input w-full font-mono input-sm"
                placeholder="Overall reason for this batch"
                bind:value={reason}
              />
            </FormField>
          </div>
          <button
            class="btn font-mono btn-primary btn-sm"
            disabled={loading}
            onclick={handlePreview}
          >
            {#if loading}
              <span class="loading loading-xs loading-spinner"></span>
            {/if}
            Preview Changes
          </button>
        </div>
      {/if}
    </div>

    <!-- Step 2: Review -->
  {:else if step === 'review' && plan}
    {#if loading}
      <LoadingSpinner />
    {:else}
      <PlanReview
        {plan}
        onApply={handleApply}
        onCancel={handleCancel}
        {applying}
      />
    {/if}

    <!-- Step 3: Result -->
  {:else if step === 'result'}
    <div class="rounded border border-base-300 bg-base-100 p-6 text-center">
      {#if applyResult?.success}
        <div class="mb-2 text-4xl text-success">&#10003;</div>
        <h2 class="font-mono text-lg font-bold text-success">
          Changes Applied
        </h2>
        <p class="font-prose mt-1 text-sm text-base-content/60">
          All operations were applied successfully.
        </p>
        {#if applyResult.auditId}
          <a
            href={link(`/write/audit/${applyResult.auditId}`)}
            class="btn mt-3 border-base-300 btn-outline font-mono btn-sm"
          >
            View Audit Entry
          </a>
        {/if}
      {:else}
        <div class="mb-2 text-4xl text-error">&#10007;</div>
        <h2 class="font-mono text-lg font-bold text-error">Apply Failed</h2>
        <p class="font-prose mt-1 text-sm text-base-content/60">
          {applyResult?.error || 'Unknown error'}
        </p>
      {/if}
      <button
        class="btn mt-4 font-mono btn-primary btn-sm"
        onclick={handleReset}
      >
        New Operation
      </button>
    </div>
  {/if}
</PageContainer>
