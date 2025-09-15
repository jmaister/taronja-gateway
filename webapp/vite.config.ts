/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from "path";


// https://vite.dev/config/
export default defineConfig({
    base: '/_/admin/',
    plugins: [
        react(),
        tailwindcss(),
    ],
    resolve: {
        alias: {
            "@": path.resolve(__dirname, "./src"),
        },
    },
    build: {
        rollupOptions: {
            output: {
                manualChunks: {
                    // Separate heavy mapping libraries into their own chunk
                    'maplibre': ['maplibre-gl'],
                    // Separate chart libraries
                    'charts': ['chart.js', 'react-chartjs-2'],
                    // React and core libraries
                    'react-vendor': ['react', 'react-dom', 'react-router-dom'],
                }
            }
        },
        chunkSizeWarningLimit: 1000, // Increase warning limit for map chunks
    },
    test: {
        globals: true,
        environment: 'jsdom',
        setupFiles: './src/test/setup.ts',
    },
});
