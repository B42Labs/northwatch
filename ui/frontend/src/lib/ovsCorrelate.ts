// Pure helpers for the OVS↔OVN correlation shown on the OVS interface detail
// view. They classify a correlation into a headline status badge and map the
// bound port's up state to a variant. Kept free of Svelte so the logic is
// unit-testable in isolation (mirrors lib/ovsDetail.ts, lib/ovsRefs.ts).

import type { Variant } from './status';
import type { OvsInterfaceCorrelation } from './api';

/** correlationStatus classifies a correlation into a headline badge: whether the
 * interface is OVN-managed at all, has a Southbound Port_Binding, is drifting
 * (SB up but the interface down/erroring), or is bound on a different chassis. */
export function correlationStatus(c: OvsInterfaceCorrelation): {
  label: string;
  variant: Variant;
} {
  if (!c.iface_id) return { label: 'not OVN-managed', variant: 'neutral' };
  if (!c.bound || !c.binding)
    return { label: 'no SB Port_Binding', variant: 'warning' };
  if (c.drift && c.drift.length > 0)
    return { label: 'drift', variant: 'error' };
  if (!c.binding.bound_here && c.binding.chassis)
    return { label: `bound on ${c.binding.chassis}`, variant: 'warning' };
  return { label: 'bound', variant: 'success' };
}

/** upVariant maps a Port_Binding up state to a status variant. */
export function upVariant(up?: boolean): Variant {
  return up ? 'success' : 'neutral';
}
