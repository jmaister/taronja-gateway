/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: 'rgb(var(--color-bg) / <alpha-value>)',
        fg: 'rgb(var(--color-fg) / <alpha-value>)',
        muted: 'rgb(var(--color-muted) / <alpha-value>)',
        'muted-fg': 'rgb(var(--color-muted-fg) / <alpha-value>)',
        surface: 'rgb(var(--color-surface) / <alpha-value>)',
        'surface-2': 'rgb(var(--color-surface-2) / <alpha-value>)',
        border: 'rgb(var(--color-border) / <alpha-value>)',
        ring: 'rgb(var(--color-ring) / <alpha-value>)',
        primary: 'rgb(var(--color-primary) / <alpha-value>)',
        'primary-fg': 'rgb(var(--color-primary-fg) / <alpha-value>)',
        danger: 'rgb(var(--color-danger) / <alpha-value>)',
        'danger-fg': 'rgb(var(--color-danger-fg) / <alpha-value>)',
        warning: 'rgb(var(--color-warning) / <alpha-value>)',
        'warning-fg': 'rgb(var(--color-warning-fg) / <alpha-value>)',
        success: 'rgb(var(--color-success) / <alpha-value>)',
        'success-fg': 'rgb(var(--color-success-fg) / <alpha-value>)',
      },
      borderRadius: {
        xl: '0.9rem',
      },
      boxShadow: {
        soft: '0 1px 2px rgb(0 0 0 / 0.06), 0 10px 28px rgb(0 0 0 / 0.08)',
      },
    },
  },
  plugins: [],
}

