import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  define: { __APP_VERSION__: JSON.stringify('test') },
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts', 'mock/**/*.test.ts'],
    setupFiles: ['src/test/setup.ts'],
    restoreMocks: true,
    // Node ≥ 25 enables its own localStorage global, which shadows jsdom's.
    execArgv: ['--no-experimental-webstorage'],
  },
})
