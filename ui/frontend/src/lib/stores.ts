import { writable } from 'svelte/store';

export type Theme = 'northwatch-dark' | 'northwatch-light';

const STORAGE_KEY = 'northwatch-theme';

// Map legacy stored values ('light' / 'dark') onto the named Console themes.
const LEGACY: Record<string, Theme> = {
  light: 'northwatch-light',
  dark: 'northwatch-dark',
};

function getStoredTheme(): Theme {
  if (typeof localStorage === 'undefined') return 'northwatch-dark';
  const raw = localStorage.getItem(STORAGE_KEY) ?? '';
  if (raw === 'northwatch-light' || raw === 'northwatch-dark') return raw;
  return LEGACY[raw] ?? 'northwatch-dark';
}

export const theme = writable<Theme>(getStoredTheme());

theme.subscribe((value) => {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', value);
    localStorage.setItem(STORAGE_KEY, value);
  }
});
