/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import path from "path";
import { visualizer } from 'rollup-plugin-visualizer';

function manualChunks(id: string): string | undefined {
    const normalizedId = id.replaceAll('\\', '/');

    if (!normalizedId.includes('/node_modules/')) {
        return undefined;
    }

    // Keep vendor chunking compatible with Vite 8's function-based manualChunks.
    if (normalizedId.includes('/node_modules/maplibre-gl/')) {
        return 'maplibre-gl';
    }

    if (normalizedId.includes('/node_modules/react-map-gl/')) {
        return 'react-map-gl';
    }

    if (
        normalizedId.includes('/node_modules/chart.js/') ||
        normalizedId.includes('/node_modules/react-chartjs-2/')
    ) {
        return 'charts';
    }

    if (
        normalizedId.includes('/node_modules/react/') ||
        normalizedId.includes('/node_modules/react-dom/') ||
        normalizedId.includes('/node_modules/react-router-dom/') ||
        normalizedId.includes('/node_modules/@tanstack/react-query/')
    ) {
        return 'react-vendor';
    }

    return undefined;
}

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
                manualChunks,
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
