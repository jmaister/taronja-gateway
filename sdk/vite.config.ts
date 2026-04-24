import { defineConfig } from 'vite';

export default defineConfig({
    build: {
        emptyOutDir: true,
        lib: {
            entry: 'src/index.ts',
            formats: ['es', 'cjs'],
            fileName: (format) => format === 'es' ? 'index.js' : 'index.cjs',
        },
        sourcemap: true,
        rollupOptions: {
            external: ['react', 'react/jsx-runtime'],
        },
    },
});