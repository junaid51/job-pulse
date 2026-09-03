import { defineConfig, devices } from '@playwright/test'

/** The app is a phone-first PWA, so the phone viewport is the default and the
 *  desktop one is the exception — not the other way round. WebKit is here
 *  because the only device that matters in practice is an iPhone. */
export default defineConfig({
  testDir: './tests',
  fullyParallel: false, // one backend, one corpus: tests share hunt state
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list']],
  timeout: 45_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:4173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'iphone', use: { ...devices['iPhone 13'] } },
    { name: 'android', use: { ...devices['Pixel 7'] } },
  ],
})
