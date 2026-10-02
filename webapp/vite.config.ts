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
        dedupe: ['react', 'react-dom'],
        alias: {
            '@': path.resolve(import.meta.dirname, './src'),
            react: path.resolve(import.meta.dirname, './node_modules/react'),
            'react/jsx-runtime': path.resolve(import.meta.dirname, './node_modules/react/jsx-runtime.js'),
            'react/jsx-dev-runtime': path.resolve(import.meta.dirname, './node_modules/react/jsx-dev-runtime.js'),
            'react-dom': path.resolve(import.meta.dirname, './node_modules/react-dom'),
            'react-dom/client': path.resolve(import.meta.dirname, './node_modules/react-dom/client.js'),
        },
    },
    build: {
        rollupOptions: {
            output: {
                manualChunks,
            }
        },
        // maplibre-gl (a full WebGL map renderer) is already isolated into its
        // own chunk (manualChunks above) and already lazy-loaded via
        // React.lazy()/dynamic import() (see LazyRequestsWorldMap.tsx) — it's
        // only fetched when a page that renders the map is actually visited,
        // and its gzipped size (~270 kB) is what real users pay for. There's
        // no further splitting to be done on a single vendor library, so the
        // warning limit is raised past its current built size (~1030 kB
        // uncompressed) with headroom for minor maplibre-gl version bumps,
        // rather than chasing an unactionable warning on every build.
        chunkSizeWarningLimit: 1200,
    },
    test: {
        globals: true,
        environment: 'jsdom',
        setupFiles: './src/test/setup.ts',
    },
}));
