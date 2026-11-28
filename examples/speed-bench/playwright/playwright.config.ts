import { defineConfig } from 'playwright/test';

// The same settings as ../manul/manul.config.json: system Chrome, headless,
// one worker, no retries, 5s for an element or an assertion, 30s for a
// navigation.
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 5_000 },
  reporter: 'list',
  use: {
    baseURL: process.env.SITE ?? 'http://localhost:8000',
    channel: 'chrome',
    headless: true,
    locale: 'en-US',
    actionTimeout: 5_000,
    navigationTimeout: 30_000,
    screenshot: 'only-on-failure',
    trace: 'off',
    video: 'off',
  },
});
