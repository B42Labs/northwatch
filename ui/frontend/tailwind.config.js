/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{svelte,ts}'],
  theme: {
    extend: {
      fontFamily: {
        // Monospace is the workhorse: data, labels, chrome — the terminal voice.
        mono: [
          '"JetBrains Mono"',
          'ui-monospace',
          'SFMono-Regular',
          'Menlo',
          'Consolas',
          'monospace',
        ],
        // Sans is used only for longer descriptive prose.
        sans: ['"IBM Plex Sans"', 'ui-sans-serif', 'system-ui', 'sans-serif'],
      },
      fontSize: {
        '2xs': ['0.6875rem', { lineHeight: '1rem' }],
      },
      letterSpacing: {
        terminal: '0.02em',
      },
      keyframes: {
        'nw-pulse-ring': {
          '0%': { boxShadow: '0 0 0 0 var(--nw-pulse, rgba(74, 222, 128, 0.5))' },
          '70%': { boxShadow: '0 0 0 6px rgba(74, 222, 128, 0)' },
          '100%': { boxShadow: '0 0 0 0 rgba(74, 222, 128, 0)' },
        },
        'nw-row-flash': {
          '0%': { backgroundColor: 'var(--nw-flash, rgba(245, 185, 71, 0.22))' },
          '100%': { backgroundColor: 'transparent' },
        },
      },
      animation: {
        'nw-pulse-ring': 'nw-pulse-ring 2s ease-out infinite',
        'nw-row-flash': 'nw-row-flash 2s ease-out',
      },
    },
  },
  plugins: [require('daisyui')],
  daisyui: {
    logs: false,
    themes: [
      {
        // ── Console / Terminal · dark (default) ─────────────────────────────
        // Warm near-black to avoid OLED halation; dual-phosphor green + amber.
        'northwatch-dark': {
          'color-scheme': 'dark',
          primary: '#4ade80', // phosphor green — live / healthy / brand
          'primary-content': '#06140b',
          secondary: '#f5b947', // amber — attention / warning accent
          'secondary-content': '#1b1402',
          accent: '#5ad1de', // cyan — info, used sparingly
          'accent-content': '#04181c',
          neutral: '#1a1e25',
          'neutral-content': '#c8d1da',
          'base-100': '#111418', // panels & cards
          'base-200': '#0b0d10', // app background (darker)
          'base-300': '#232934', // hairline borders & hover
          'base-content': '#ccd4dd',
          info: '#5ad1de',
          success: '#45d07a',
          warning: '#f5b947',
          error: '#fb6a6a',
          '--rounded-box': '0.25rem',
          '--rounded-btn': '0.25rem',
          '--rounded-badge': '0.125rem',
          '--tab-radius': '0.25rem',
          '--border-btn': '1px',
          '--animation-btn': '0.15s',
          '--animation-input': '0.15s',
        },
      },
      {
        // ── Console / Terminal · light (modern light-code-editor) ───────────
        'northwatch-light': {
          'color-scheme': 'light',
          primary: '#1f9d57',
          'primary-content': '#ffffff',
          secondary: '#b5790b',
          'secondary-content': '#ffffff',
          accent: '#1c7e8c',
          'accent-content': '#ffffff',
          neutral: '#e4e7e0',
          'neutral-content': '#23272e',
          'base-100': '#f9faf7',
          'base-200': '#edefe9',
          'base-300': '#d6dad0',
          'base-content': '#14171a',
          info: '#1c7e8c',
          success: '#1f9d57',
          warning: '#b5790b',
          error: '#c2403f',
          '--rounded-box': '0.25rem',
          '--rounded-btn': '0.25rem',
          '--rounded-badge': '0.125rem',
          '--tab-radius': '0.25rem',
          '--border-btn': '1px',
          '--animation-btn': '0.15s',
          '--animation-input': '0.15s',
        },
      },
    ],
  },
};
