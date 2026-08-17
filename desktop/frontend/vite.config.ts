import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig(({ mode }) => {
  const web = mode === 'web';
  return {
    define: {
      'import.meta.env.VITE_WEB': JSON.stringify(web ? 'true' : ''),
    },
    plugins: [svelte()],
    resolve: {
      conditions: ['browser'],
    },
    server: {
      host: '127.0.0.1',
      port: web ? 5173 : Number(process.env.WAILS_VITE_PORT) || 9245,
      strictPort: !web,
      proxy: web ? { '/api': 'http://127.0.0.1:8787' } : undefined,
    },
    assetsInclude: ['**/*.wasm'],
    build: {
      target: 'es2022',
      outDir: web ? 'dist-web' : 'dist',
      emptyOutDir: true,
    },
    test: {
      environment: 'jsdom',
      setupFiles: ['./src/test/setup.ts'],
      css: true,
    },
  };
});
