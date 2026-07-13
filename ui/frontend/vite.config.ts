import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { svelteTesting } from '@testing-library/svelte/vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  // svelteTesting() is a no-op outside Vitest (it early-returns unless
  // process.env.VITEST is set), so it does not affect `vite build`.
  plugins: [tailwindcss(), svelte(), svelteTesting()],
  server: {
    proxy: {
      '/api/v1/ws': {
        target: 'ws://localhost:8080',
        ws: true,
      },
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
      '/readyz': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
  },
});
