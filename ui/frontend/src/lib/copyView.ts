import { get, writable } from 'svelte/store';
import { location as routeLocation, parseQuery } from './router';
import { activeClusterInfo } from './clusterStore';
import { appMode, snapshotInfo } from './capabilitiesStore';

/**
 * copyView turns the data a page currently shows into something pasteable into
 * an external AI assistant: Markdown with a context header, or raw JSON. The
 * context is assembled here because the API writes bare payloads with no
 * envelope, so cluster, mode and time are only known in the browser.
 */

/**
 * copyText puts text on the clipboard. The async Clipboard API needs a secure
 * context, and Northwatch is commonly served over plain HTTP on a lab host, so
 * the textarea + execCommand path is the one that actually runs there rather
 * than a nicety.
 */
export async function copyText(text: string): Promise<void> {
  if (window.isSecureContext && navigator.clipboard) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Chrome rejects with NotAllowedError when the document is not focused.
      // Fall through to the synchronous path, which does not care.
    }
  }
  copyViaTextarea(text);
}

function copyViaTextarea(text: string): void {
  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.top = '-1000px';
  document.body.appendChild(area);
  try {
    area.select();
    if (!document.execCommand('copy')) {
      throw new Error('clipboard unavailable');
    }
  } finally {
    document.body.removeChild(area);
  }
}

export interface ViewContext {
  title: string;
  /** Active cluster name, 'default' when no cluster is registered yet. */
  cluster: string;
  /** Human-readable cluster label, empty when unknown. */
  clusterLabel: string;
  mode: 'live' | 'snapshot';
  /** Creation time of the backing snapshot, snapshot mode only. */
  snapshotCreatedAt?: string;
  /** Hash route without its query part, e.g. '/debug/port-diagnostics'. */
  route: string;
  query: Record<string, string>;
  copiedAt: string;
  masked: boolean;
}

/** viewContext captures who, where and when for a copied view. */
export function viewContext(title: string, masked: boolean): ViewContext {
  const cluster = get(activeClusterInfo);
  const mode = get(appMode);
  const path = get(routeLocation);
  const sep = path.indexOf('?');

  const ctx: ViewContext = {
    title,
    cluster: cluster?.name ?? 'default',
    clusterLabel: cluster?.label ?? '',
    mode,
    route: sep === -1 ? path : path.slice(0, sep),
    query: sep === -1 ? {} : parseQuery(path.slice(sep + 1)),
    copiedAt: new Date().toISOString(),
    masked,
  };

  if (mode === 'snapshot') {
    const createdAt = get(snapshotInfo)?.createdAt;
    if (createdAt) ctx.snapshotCreatedAt = createdAt;
  }

  return ctx;
}

/** renderJSON serializes a view for a tool that wants machine-readable input. */
export function renderJSON(data: unknown): string {
  return JSON.stringify(data, null, 2) ?? 'null';
}

/** renderMarkdown wraps a view in the context header, for pasting into a chat. */
export function renderMarkdown(ctx: ViewContext, data: unknown): string {
  const lines = [
    `## Northwatch view: ${ctx.title}`,
    '',
    `- Cluster: ${ctx.cluster}${ctx.clusterLabel ? ` (${ctx.clusterLabel})` : ''}`,
    `- Mode: ${renderMode(ctx)}`,
    `- Route: ${ctx.route}`,
    `- Query: ${renderQuery(ctx.query)}`,
    `- Copied at: ${ctx.copiedAt}`,
    `- Masked: ${ctx.masked ? 'yes' : 'no'}`,
    '',
    '```json',
    renderJSON(data),
    '```',
  ];
  return lines.join('\n');
}

function renderMode(ctx: ViewContext): string {
  if (ctx.mode !== 'snapshot') return 'live';
  return ctx.snapshotCreatedAt
    ? `snapshot (created ${ctx.snapshotCreatedAt})`
    : 'snapshot';
}

function renderQuery(query: Record<string, string>): string {
  const entries = Object.entries(query);
  if (entries.length === 0) return '(none)';
  return entries.map(([k, v]) => `${k}=${v}`).join(', ');
}

// MAC runs before IPv6 because an IPv6 pattern happily matches
// 00:11:22:33:44:55. IPv6 accepts the full eight-group form or anything
// containing '::', which keeps a timestamp like 10:00:00 out of the match.
const MAC_RE = /\b(?:[0-9a-f]{2}:){5}[0-9a-f]{2}\b/gi;
const IP6_RE =
  /(?<![0-9a-f:])(?:(?:[0-9a-f]{1,4}:){7}[0-9a-f]{1,4}|(?:[0-9a-f]{1,4}:){0,6}(?:[0-9a-f]{1,4})?::(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,6})?)(?![0-9a-f:])/gi;
const IP4_RE = /\b(?:\d{1,3}\.){3}\d{1,3}\b/g;

/**
 * maskSensitive replaces addresses with stable placeholders so a view from a
 * customer environment can be pasted without hand-editing it. It is a
 * convenience, not a guarantee: only addresses and `hostname` values are
 * rewritten, because OVN entity names cannot be told apart from hostnames.
 */
export function maskSensitive(data: unknown): unknown {
  const seen = new Map<string, string>();
  const counters = { mac: 0, ip6: 0, ip: 0, host: 0 };

  const placeholder = (kind: keyof typeof counters, value: string): string => {
    const key = `${kind}:${value.toLowerCase()}`;
    const existing = seen.get(key);
    if (existing) return existing;
    counters[kind] += 1;
    const made = `${kind}-${counters[kind]}`;
    seen.set(key, made);
    return made;
  };

  const maskString = (value: string): string =>
    value
      .replace(MAC_RE, (m) => placeholder('mac', m))
      .replace(IP6_RE, (m) => placeholder('ip6', m))
      .replace(IP4_RE, (m) => placeholder('ip', m));

  const walk = (value: unknown, key?: string): unknown => {
    if (typeof value === 'string') {
      return key === 'hostname' && value
        ? placeholder('host', value)
        : maskString(value);
    }
    if (Array.isArray(value)) return value.map((item) => walk(item));
    if (value && typeof value === 'object') {
      const out: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
        out[k] = walk(v, k);
      }
      return out;
    }
    return value;
  };

  return walk(data);
}

const MASK_STORAGE_KEY = 'northwatch-copy-mask';

function storedMask(): boolean {
  try {
    return localStorage.getItem(MASK_STORAGE_KEY) === '1';
  } catch {
    // Private-mode browsers throw on storage access; default to off.
    return false;
  }
}

/** maskEnabled remembers the operator's masking choice across page loads. */
export const maskEnabled = writable<boolean>(storedMask());

maskEnabled.subscribe((value) => {
  try {
    localStorage.setItem(MASK_STORAGE_KEY, value ? '1' : '0');
  } catch {
    // Not persistable; the choice holds for this page only.
  }
});
