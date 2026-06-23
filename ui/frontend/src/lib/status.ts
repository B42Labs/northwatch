// Central presentation mapping for statuses, event actions, and severities.
// Previously this switch logic was copy-pasted across 6+ components.

export type Variant =
  | 'neutral'
  | 'primary'
  | 'secondary'
  | 'accent'
  | 'success'
  | 'warning'
  | 'error'
  | 'info'
  | 'ghost';

/** DaisyUI badge class for a variant. */
export const badgeClass: Record<Variant, string> = {
  neutral: 'badge-neutral',
  primary: 'badge-primary',
  secondary: 'badge-secondary',
  accent: 'badge-accent',
  success: 'badge-success',
  warning: 'badge-warning',
  error: 'badge-error',
  info: 'badge-info',
  ghost: 'badge-ghost',
};

/** Text color class for a variant. */
export const textClass: Record<Variant, string> = {
  neutral: 'text-base-content',
  primary: 'text-primary',
  secondary: 'text-secondary',
  accent: 'text-accent',
  success: 'text-success',
  warning: 'text-warning',
  error: 'text-error',
  info: 'text-info',
  ghost: 'text-base-content/60',
};

/** Left-accent border class for a variant (used by Card / chain segments). */
export const accentBorderClass: Record<Variant, string> = {
  neutral: 'border-l-base-300',
  primary: 'border-l-primary',
  secondary: 'border-l-secondary',
  accent: 'border-l-accent',
  success: 'border-l-success',
  warning: 'border-l-warning',
  error: 'border-l-error',
  info: 'border-l-info',
  ghost: 'border-l-base-300',
};

// ── Event / write actions (insert/update/delete) ─────────────────────────

export function actionVariant(action: string): Variant {
  switch (action.toLowerCase()) {
    case 'insert':
    case 'create':
    case 'add':
      return 'success';
    case 'update':
    case 'modify':
    case 'mutate':
      return 'warning';
    case 'delete':
    case 'remove':
      return 'error';
    default:
      return 'neutral';
  }
}

/** Single-glyph marker for an action, terminal-diff style. */
export function actionGlyph(action: string): string {
  switch (action.toLowerCase()) {
    case 'insert':
    case 'create':
    case 'add':
      return '+';
    case 'update':
    case 'modify':
    case 'mutate':
      return '~';
    case 'delete':
    case 'remove':
      return '-';
    default:
      return '·';
  }
}

// ── Severities (audit / diagnostics) ─────────────────────────────────────

export function severityVariant(severity: string): Variant {
  switch (severity.toLowerCase()) {
    case 'critical':
    case 'high':
    case 'error':
    case 'danger':
      return 'error';
    case 'medium':
    case 'warning':
    case 'warn':
      return 'warning';
    case 'low':
    case 'info':
    case 'notice':
      return 'info';
    case 'ok':
    case 'pass':
    case 'healthy':
      return 'success';
    default:
      return 'neutral';
  }
}

// ── WebSocket / cluster connection state ─────────────────────────────────

export type ConnState = 'connected' | 'connecting' | 'disconnected';

export function connVariant(state: ConnState): Variant {
  return state === 'connected'
    ? 'success'
    : state === 'connecting'
      ? 'warning'
      : 'error';
}

export function connLabel(state: ConnState): string {
  return state === 'connected'
    ? 'live'
    : state === 'connecting'
      ? 'linking'
      : 'offline';
}
