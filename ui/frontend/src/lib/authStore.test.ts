import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';
import { apiToken, getApiToken, setApiToken, clearApiToken } from './authStore';

// The store keeps the token in sessionStorage; the tests supply their own so
// they do not depend on the test environment's implementation.
function fakeStorage(): Storage {
  const data = new Map<string, string>();
  return {
    get length() {
      return data.size;
    },
    clear: () => data.clear(),
    getItem: (k: string) => data.get(k) ?? null,
    key: (i: number) => [...data.keys()][i] ?? null,
    removeItem: (k: string) => data.delete(k),
    setItem: (k: string, v: string) => data.set(k, v),
  } as Storage;
}

describe('authStore', () => {
  beforeEach(() => {
    vi.stubGlobal('sessionStorage', fakeStorage());
    clearApiToken();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('starts empty when nothing is stored', () => {
    expect(getApiToken()).toBe('');
    expect(get(apiToken)).toBe('');
  });

  it('persists the token so an operator pastes it once', () => {
    setApiToken('0123456789abcdef');

    expect(getApiToken()).toBe('0123456789abcdef');
    expect(get(apiToken)).toBe('0123456789abcdef');
    expect(sessionStorage.getItem('northwatch_api_token')).toBe(
      '0123456789abcdef',
    );
  });

  it('trims surrounding whitespace from a pasted token', () => {
    setApiToken('  0123456789abcdef \n');

    expect(getApiToken()).toBe('0123456789abcdef');
  });

  it('clearing removes the stored token', () => {
    setApiToken('0123456789abcdef');
    clearApiToken();

    expect(getApiToken()).toBe('');
    expect(sessionStorage.getItem('northwatch_api_token')).toBeNull();
  });

  it('setting an empty token clears rather than storing a blank value', () => {
    setApiToken('0123456789abcdef');
    setApiToken('   ');

    expect(getApiToken()).toBe('');
    expect(sessionStorage.getItem('northwatch_api_token')).toBeNull();
  });

  it('survives a browser that refuses sessionStorage access', () => {
    // Private-mode browsers throw on access; the token must still work for the
    // session rather than taking the app down.
    vi.stubGlobal('sessionStorage', {
      getItem: () => {
        throw new Error('access denied');
      },
      setItem: () => {
        throw new Error('access denied');
      },
      removeItem: () => {
        throw new Error('access denied');
      },
    });

    expect(() => setApiToken('0123456789abcdef')).not.toThrow();
    expect(getApiToken()).toBe('0123456789abcdef');
  });
});
