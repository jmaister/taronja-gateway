/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter var', 'ui-sans-serif', 'system-ui', 'sans-serif'], // Keep a good sans-serif default
        serif: ['Georgia', 'Times New Roman', 'Times', 'serif'],
      },
    },
  },
}
