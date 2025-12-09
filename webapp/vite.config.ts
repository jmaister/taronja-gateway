/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from "path";
import { visualizer } from 'rollup-plugin-visualizer';

// https://vite.dev/config/
export default defineConfig(({ mode }) => ({
    base: '/_/admin/',
    plugins: [
        react(),
        tailwindcss(),
        // Visualizer plugin to analyze bundle size
        mode === 'analyze' && visualizer({
            open: true,
            filename: 'dist/bundle-stats.html',
            gzipSize: true,
            brotliSize: true,
        }),
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
                    'maplibre-gl': ['maplibre-gl'],
                    'react-map-gl': ['react-map-gl/maplibre'],
                    // Separate chart libraries
                    'charts': ['chart.js', 'react-chartjs-2'],
                    // React and core libraries
                    'react-vendor': ['react', 'react-dom', 'react-router-dom', '@tanstack/react-query'],
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
}));
