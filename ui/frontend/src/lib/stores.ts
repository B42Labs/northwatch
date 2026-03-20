import { writable } from 'svelte/store';

function getStoredTheme(): string {
  if (typeof localStorage !== 'undefined') {
    return localStorage.getItem('northwatch-theme') || 'light';
  }
  return 'light';
}

export const theme = writable(getStoredTheme());

theme.subscribe((value) => {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', value);
    localStorage.setItem('northwatch-theme', value);
  }
});

export const sidebarOpen = writable(true);
