import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: '/humidity/', // where the gateway serves this app
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    host: true,
    port: 5173,
    // Phones reach the dev server through the tailnet name (*.ts.net).
    allowedHosts: ['.ts.net'],
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
