/**
 * Playwright Configuration for XPB Browser Testing
 * 
 * Tests run in both Chrome and Firefox to compare performance
 */

import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './benchmarks',
  
  // Run tests in both Chrome and Firefox
  projects: [
    {
      name: 'chrome',
      use: {
        ...devices['Desktop Chrome'],
        channel: 'chrome',
        launchOptions: {
          executablePath: process.env.CHROME_PATH || '/usr/bin/google-chrome-stable',
        },
      },
    },
    {
      name: 'firefox',
      use: {
        ...devices['Desktop Firefox'],
        launchOptions: {
          executablePath: process.env.FIREFOX_PATH || '/usr/bin/firefox',
        },
      },
    },
  ],

  // Web server to serve the test page
  webServer: {
    command: 'node benchmarks/test-server.js',
    port: 8765,
    reuseExistingServer: true,
  },

  // Reporter configuration
  reporter: [
    ['list'],
    ['html', { outputFolder: 'benchmarks/playwright-report' }],
    ['json', { outputFile: 'benchmarks/results/browser-results.json' }],
  ],

  // Test timeout (benchmarks need more time)
  timeout: 300000, // 5 minutes for full benchmark suite
  
  // Expect timeout
  expect: {
    timeout: 60000,
  },

  // Workers - run browsers sequentially for consistent results
  workers: 1,

  // Retry on failure
  retries: 0,

  // Output directory
  outputDir: 'benchmarks/test-results',

  // Preserve screenshots/videos on failure
  use: {
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
});
