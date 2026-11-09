import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    host: true,
    port: 5173,
    // Phones reach the dev server through the tailnet name (*.ts.net).
    allowedHosts: ['.ts.net'],
    proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: false } },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: ['es2020', 'safari14'],
  },
})
