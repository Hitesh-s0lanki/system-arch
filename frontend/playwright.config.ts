import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 180_000,
  expect: { timeout: 15_000 },
  workers: 1,
  use: {
    baseURL: process.env.FRONTEND_URL || 'http://localhost:3000',
    viewport: { width: 1440, height: 1000 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    launchOptions: {
      executablePath: process.env.BROWSER_EXECUTABLE || undefined,
    },
  },
  webServer: {
    command: 'npm run dev',
    url: process.env.FRONTEND_URL || 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
