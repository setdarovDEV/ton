import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { fileURLToPath, URL } from 'node:url';

// Dev server proxies /api and /ws to the Go API so the SPA can use same-origin URLs.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: process.env.VITE_DEV_API_TARGET ?? 'http://localhost:8080', changeOrigin: true },
      '/healthz': { target: process.env.VITE_DEV_API_TARGET ?? 'http://localhost:8080', changeOrigin: true },
      '/ws': { target: process.env.VITE_DEV_API_TARGET ?? 'http://localhost:8080', ws: true, changeOrigin: true },
    },
  },
  build: {
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/@tonconnect')) return 'ton';
          if (/node_modules\/(react|react-dom|react-router|@tanstack)/.test(id)) return 'vendor';
          return undefined;
        },
      },
    },
  },
});
