/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx,vue}'],
  theme: {
    extend: {
      colors: {
        'blue-900': '#1e3c72',
        'blue-800': '#2a5298',
        'yellow-400': '#ffd700',
      },
      spacing: {
        '15': '3.75rem',
        '30': '7.5rem',
      }
    },
  },
  plugins: [],
}
