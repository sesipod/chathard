import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [tailwindcss(), svelte()],
  server: {
    proxy: {
      '/api': { target: 'http://localhost:3000', changeOrigin: true },
      '/ws': {
        target: 'ws://localhost:3000',
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
  resolve: {
    alias: {
      '@noble/curves/ed25519': '@noble/curves/esm/ed25519.js',
      '@noble/curves/x25519': '@noble/curves/esm/x25519.js',
    },
  },
});
