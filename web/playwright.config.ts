import { defineConfig, devices } from '@playwright/test'

// Smoke test on a phone viewport against the dev mock (no backend needed).
export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  retries: 0,
  use: {
    ...devices['Pixel 7'],
    baseURL: 'http://localhost:5174',
    trace: 'retain-on-failure',
    // The full Chromium build in headless mode (no separate headless-shell download needed).
    channel: 'chromium',
  },
  webServer: {
    command: 'npx vite --mode mock --port 5174 --strictPort',
    url: 'http://localhost:5174/api/v1/health',
    env: { MOCK_SCENARIO: 'empty' },
    reuseExistingServer: false,
    timeout: 60_000,
  },
})
