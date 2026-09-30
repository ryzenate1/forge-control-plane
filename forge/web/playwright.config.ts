import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:3000';
const mockPort = process.env.MOCK_API_PORT || '8080';
const mockUrl = `http://127.0.0.1:${mockPort}/api/v1/setup/status`;

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  expect: { timeout: 5000 },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : undefined,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  webServer: [
    {
      command: `node e2e/mock-server.mjs`,
      url: mockUrl,
      timeout: 15_000,
      reuseExistingServer: !process.env.CI,
      env: { MOCK_API_PORT: mockPort },
    },
    {
      command: 'npm run build && npm run start',
      url: baseURL,
      timeout: 120_000,
      reuseExistingServer: !process.env.CI,
      env: { API_INTERNAL_URL: `http://127.0.0.1:${mockPort}`, NEXT_PUBLIC_API_URL: '/api/v1' },
    },
  ],
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
