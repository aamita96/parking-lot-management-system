import { defineConfig, devices } from '@playwright/test'

// End-to-end tests drive the real stack (NFR-16). Start it first with `docker compose up`,
// or point E2E_BASE_URL at the Vite dev server (http://localhost:5173).
export default defineConfig({
  testDir: './e2e',
  timeout: 90_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  reporter: [['list']],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:3000',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 960 } } },
  ],
})
