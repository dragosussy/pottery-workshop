import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: '/humidity/', // where the gateway serves this app
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      '@ui': fileURLToPath(new URL('../../../shared/ui', import.meta.url)),
    },
    // shared/ui has no node_modules of its own, so it uses this app's Vue.
    dedupe: ['vue'],
  },
  server: {
    host: true,
    port: 5173,
    // Phones reach the dev server through the tailnet name (*.ts.net).
    allowedHosts: ['.ts.net'],
    fs: { allow: ['../../..'] }, // shared/ui is outside this folder
    // Strips the prefix, as the gateway does on the Pi.
    proxy: {
      '/humidity/api': {
        target: 'http://localhost:8080',
        changeOrigin: false,
        rewrite: (path) => path.replace(/^\/humidity/, ''),
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: ['es2020', 'safari14'],
  },
})
