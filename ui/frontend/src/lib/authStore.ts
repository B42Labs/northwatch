import { get, writable } from 'svelte/store';

const STORAGE_KEY = 'northwatch_api_token';

function initial(): string {
  try {
    return sessionStorage.getItem(STORAGE_KEY) ?? '';
  } catch {
    // Private-mode browsers throw on sessionStorage access; the token simply
    // does not persist across reloads there.
    return '';
  }
}

/**
 * apiToken holds the bearer token sent with every mutating API request. It is
 * kept in sessionStorage — scoped to this tab and cleared when it closes — so an
 * operator pastes it once per session rather than on every page load, without it
 * persisting indefinitely the way localStorage would. The SPA's strict CSP is
 * what keeps it out of reach of injected script.
 */
export const apiToken = writable<string>(initial());

/** getApiToken returns the token to send, or '' when none is configured. */
export function getApiToken(): string {
  return get(apiToken);
}

export function setApiToken(token: string): void {
  const trimmed = token.trim();
  try {
    if (trimmed) {
      sessionStorage.setItem(STORAGE_KEY, trimmed);
    } else {
      sessionStorage.removeItem(STORAGE_KEY);
    }
  } catch {
    // Not persistable; keep it for this session only.
  }
  apiToken.set(trimmed);
}

export function clearApiToken(): void {
  setApiToken('');
}
