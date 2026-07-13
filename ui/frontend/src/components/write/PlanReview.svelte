<script lang="ts">
  import type { Plan } from '../../lib/writeApi';
  import type { Variant } from '../../lib/status';
  import Badge from '../ui/Badge.svelte';
  import Card from '../ui/Card.svelte';
  import ImpactWarning from './ImpactWarning.svelte';
  import PlanDiffView from './PlanDiffView.svelte';

  let {
    plan,
    onApply,
    onCancel,
    applying = false,
  }: {
    plan: Plan;
    onApply: () => void;
    onCancel: () => void;
    applying?: boolean;
  } = $props();

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

  function statusVariant(status: string): Variant {
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
  <Card>
    <div class="flex flex-col gap-2">
      <div class="flex flex-wrap items-center gap-3">
        <h3
          class="font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
        >
          Plan
        </h3>
        <Badge text={plan.status} variant={statusVariant(plan.status)} />
        <span class="font-mono text-xs text-base-content/50">
          {plan.id.slice(0, 12)}
        </span>
        <span class="font-mono text-xs text-base-content/50">
          Expires in: <span class:text-error={expired}>{remaining}</span>
        </span>
      </div>
      <div class="font-mono text-xs text-base-content/60">
        {plan.operations.length} operation(s) &middot; {plan.diffs.length} change(s)
      </div>
    </div>
  </Card>

  <!-- Impact warning -->
  {#if plan.impact && plan.impact.length > 0}
    <ImpactWarning entries={plan.impact} />
  {/if}

  <!-- Diffs -->
  <div>
    <h3
      class="mb-2 font-mono text-xs font-semibold tracking-wider text-base-content/80 uppercase"
    >
      Changes Preview
    </h3>
    <PlanDiffView diffs={plan.diffs} />
  </div>

  <!-- Apply controls -->
  {#if plan.status === 'pending'}
    <Card>
      <div class="flex flex-col gap-2">
        <div class="flex flex-wrap items-end gap-3">
          <button
            class="btn font-mono btn-primary btn-sm"
            disabled={expired || applying}
            onclick={() => onApply()}
          >
            {#if applying}
              <span class="loading loading-xs loading-spinner"></span>
            {/if}
            Apply Changes
          </button>
          <button
            class="btn border-base-300 btn-ghost font-mono btn-sm"
            disabled={applying}
            onclick={onCancel}
          >
            Cancel
          </button>
        </div>
        {#if expired}
          <div
            role="alert"
            class="rounded border border-error/40 bg-error/10 px-3 py-2 font-mono text-sm text-error"
          >
            Plan has expired. Preview again to create a new plan.
          </div>
        {/if}
      </div>
    </Card>
  {/if}
</div>
