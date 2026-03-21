<script lang="ts">
  import type { Plan } from '../../lib/writeApi';
  import Badge from '../ui/Badge.svelte';
  import PlanDiffView from './PlanDiffView.svelte';

  let {
    plan,
    onApply,
    onCancel,
    applying = false,
  }: {
    plan: Plan;
    onApply: (actor: string) => void;
    onCancel: () => void;
    applying?: boolean;
  } = $props();

  let actor = $state('');
  let expired = $state(false);

  $effect(() => {
    const expiresAt = new Date(plan.expires_at).getTime();
    const check = () => {
      expired = Date.now() >= expiresAt;
    };
    check();
    const interval = setInterval(check, 1000);
    return () => clearInterval(interval);
  });

  let remaining = $derived.by(() => {
    const ms = new Date(plan.expires_at).getTime() - Date.now();
    if (ms <= 0) return 'Expired';
    const secs = Math.floor(ms / 1000);
    const mins = Math.floor(secs / 60);
    const s = secs % 60;
    return `${mins}:${String(s).padStart(2, '0')}`;
  });

  function statusVariant(
    status: string,
  ): 'success' | 'warning' | 'error' | 'info' | 'neutral' {
    switch (status) {
      case 'pending':
        return 'warning';
      case 'applied':
        return 'success';
      case 'expired':
      case 'failed':
        return 'error';
      case 'dry-run':
        return 'info';
      default:
        return 'neutral';
    }
  }
</script>

<div class="flex flex-col gap-4">
  <!-- Plan metadata -->
  <div class="card bg-base-100 shadow-sm">
    <div class="card-body p-4">
      <div class="flex flex-wrap items-center gap-3">
        <h3 class="card-title text-sm">Plan</h3>
        <Badge text={plan.status} variant={statusVariant(plan.status)} />
        <span class="font-mono text-xs text-base-content/50">
          {plan.id.slice(0, 12)}
        </span>
        <span class="text-xs text-base-content/50">
          Expires in: <span class:text-error={expired}>{remaining}</span>
        </span>
      </div>
      <div class="text-xs text-base-content/60">
        {plan.operations.length} operation(s) &middot; {plan.diffs.length} change(s)
      </div>
    </div>
  </div>

  <!-- Diffs -->
  <div>
    <h3 class="mb-2 text-sm font-semibold">Changes Preview</h3>
    <PlanDiffView diffs={plan.diffs} />
  </div>

  <!-- Apply controls -->
  {#if plan.status === 'pending'}
    <div class="card bg-base-100 shadow-sm">
      <div class="card-body p-4">
        <div class="flex flex-wrap items-end gap-3">
          <div class="form-control">
            <label class="label py-0.5" for="apply-actor">
              <span class="label-text text-xs">Actor (optional)</span>
            </label>
            <input
              id="apply-actor"
              type="text"
              class="input input-sm input-bordered w-60"
              placeholder="your-name"
              bind:value={actor}
            />
          </div>
          <button
            class="btn btn-primary btn-sm"
            disabled={expired || applying}
            onclick={() => onApply(actor)}
          >
            {#if applying}
              <span class="loading loading-spinner loading-xs"></span>
            {/if}
            Apply Changes
          </button>
          <button
            class="btn btn-ghost btn-sm"
            disabled={applying}
            onclick={onCancel}
          >
            Cancel
          </button>
        </div>
        {#if expired}
          <div role="alert" class="alert alert-error mt-2 py-2 text-xs">
            Plan has expired. Preview again to create a new plan.
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
