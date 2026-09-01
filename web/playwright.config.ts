import { defineConfig } from "@playwright/test";

const devBaseURL = "http://127.0.0.1:4173";
const previewBaseURL = "http://127.0.0.1:4174";

export default defineConfig({
  testDir: "./e2e",
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  workers: 1,
  reporter: "line",
  use: {
    browserName: "chromium",
    locale: "zh-CN",
    launchOptions: process.env.CI
      ? undefined
      : {
          executablePath:
            process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ??
            "/usr/bin/google-chrome-stable"
        },
    trace: "retain-on-failure"
  },
  projects: [
    {
      name: "journeys-dev",
      testIgnore: /render-gate\.spec\.ts/,
      use: { baseURL: devBaseURL }
    },
    {
      name: "render-gate-preview",
      testMatch: /render-gate\.spec\.ts/,
      use: { baseURL: previewBaseURL }
    }
  ],
  webServer: [
    {
      command: "npm run dev -- --host 127.0.0.1 --port 4173 --strictPort",
      url: devBaseURL,
      reuseExistingServer: false,
      stdout: "pipe",
      stderr: "pipe",
      timeout: 120_000
    },
    {
      command:
        "npm run build && npm run preview -- --host 127.0.0.1 --port 4174 --strictPort",
      url: previewBaseURL,
      reuseExistingServer: false,
      stdout: "pipe",
      stderr: "pipe",
      timeout: 120_000
    }
  ]
});
