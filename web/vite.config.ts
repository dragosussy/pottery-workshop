import { fileURLToPath, URL } from 'node:url'
import { readFileSync } from 'node:fs'
import { defineConfig, type PluginOption } from 'vite'
import vue from '@vitejs/plugin-vue'

const pkg = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf8')) as { version: string }

export default defineConfig(async ({ mode, command }) => {
  const plugins: PluginOption[] = [vue()]
  // The mock API is a dev-server-only middleware. It is loaded with a dynamic
  // import so that nothing from mock/ or src/mocks/ can reach a production build.
  const useMock = command === 'serve' && mode === 'mock'
  if (useMock) {
    const { mockApi } = await import('./mock/mockApi')
    plugins.push(mockApi())
  }
  return {
    plugins,
    define: {
      __APP_VERSION__: JSON.stringify(pkg.version),
    },
    resolve: {
      alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
    },
    server: {
      host: true,
      port: 5173,
      // Phones reach the dev server through the tailnet name (*.ts.net).
      allowedHosts: ['.ts.net'],
      proxy: useMock
        ? undefined
        : {
            '/api': { target: 'http://localhost:8080', changeOrigin: false },
          },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
      target: ['es2020', 'safari14'],
      sourcemap: false,
    },
  }
})
