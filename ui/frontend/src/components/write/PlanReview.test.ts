import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { flushSync } from 'svelte';
import PlanReview from './PlanReview.svelte';
import type { Plan } from '../../lib/writeApi';

// A pending plan; individual tests override the fields they exercise. The
// timestamps are relative to Date.now(), which the tests drive with fake timers.
function makePlan(o: Partial<Plan> = {}): Plan {
  const now = Date.now();
  return {
    id: 'plan-abcdef123456',
    created_at: new Date(now).toISOString(),
    expires_at: new Date(now + 300_000).toISOString(),
    operations: [{ action: 'update', table: 'HA_Chassis', uuid: 'u1' }],
    diffs: [],
    snapshot_id: 1,
    status: 'pending',
    apply_token: 'tok-abc',
    ...o,
  };
}

function applyButton(): HTMLButtonElement {
  return screen.getByRole('button', {
    name: 'Apply Changes',
  }) as HTMLButtonElement;
}

function cancelButton(): HTMLButtonElement {
  return screen.getByRole('button', { name: 'Cancel' }) as HTMLButtonElement;
}

const START = new Date('2026-07-13T10:00:00Z').getTime();

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(START);
});

afterEach(() => {
  vi.useRealTimers();
});

describe('PlanReview', () => {
  it('renders the status badge and remaining countdown, with Apply enabled', () => {
    render(PlanReview, {
      plan: makePlan({ expires_at: new Date(START + 90_000).toISOString() }),
      onApply: () => {},
      onCancel: () => {},
    });

    expect(screen.getByText('pending')).toBeTruthy();
    // 90s remaining renders as m:ss.
    expect(screen.getByText('1:30')).toBeTruthy();
    expect(applyButton().disabled).toBe(false);
    expect(cancelButton().disabled).toBe(false);
  });

  // The actor is derived from the API token server-side, so the review pane no
  // longer offers a client-supplied one.
  it('applies without asking for an actor', async () => {
    const onApply = vi.fn();
    render(PlanReview, { plan: makePlan(), onApply, onCancel: () => {} });

    expect(screen.queryByLabelText(/Actor/i)).toBeNull();

    await fireEvent.click(applyButton());

    expect(onApply).toHaveBeenCalled();
  });

  it('disables Apply and surfaces the expired notice once expires_at passes', () => {
    render(PlanReview, {
      plan: makePlan({ expires_at: new Date(START + 5_000).toISOString() }),
      onApply: () => {},
      onCancel: () => {},
    });
    // Ensure the mount effect (which starts the 1s expiry interval) has run.
    flushSync();
    expect(applyButton().disabled).toBe(false);
    expect(screen.queryByText(/Plan has expired/i)).toBeNull();

    // Advance past expiry; the interval callback flips `expired`.
    vi.advanceTimersByTime(6_000);
    flushSync();

    expect(applyButton().disabled).toBe(true);
    expect(screen.getByText(/Plan has expired/i)).toBeTruthy();
  });

  it('disables Apply and Cancel while an apply is in flight', () => {
    render(PlanReview, {
      plan: makePlan(),
      onApply: () => {},
      onCancel: () => {},
      applying: true,
    });

    expect(applyButton().disabled).toBe(true);
    expect(cancelButton().disabled).toBe(true);
  });

  it('hides the apply controls when the plan is no longer pending', () => {
    render(PlanReview, {
      plan: makePlan({ status: 'applied' }),
      onApply: () => {},
      onCancel: () => {},
    });

    expect(screen.getByText('applied')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Apply Changes' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Cancel' })).toBeNull();
  });
});
