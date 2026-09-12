import { defineConfig } from "@playwright/test";

const portOffset = Number.parseInt(process.env.PLAYWRIGHT_PORT_OFFSET ?? "0", 10);
const devPort = 4173 + portOffset;
const previewPort = 4174 + portOffset;
const trialPort = 4175 + portOffset;
const devBaseURL = `http://127.0.0.1:${devPort}`;
const previewBaseURL = `http://127.0.0.1:${previewPort}`;
const trialBaseURL = `http://127.0.0.1:${trialPort}`;

export default defineConfig({
  testDir: "./e2e",
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  workers: process.env.CI ? 2 : 1,
  reporter: process.env.CI
    ? [
        ["line"],
        ["github"],
        ["html", { open: "never", outputFolder: "playwright-report" }]
      ]
    : "line",
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
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "journeys-dev",
      testIgnore: /render-gate\.spec\.ts|entry-production\.spec\.ts|questions-trial\.spec\.ts/,
      use: { baseURL: devBaseURL }
    },
    {
      name: "render-gate-preview",
      testMatch: /render-gate\.spec\.ts|entry-production\.spec\.ts|offline-assets\.spec\.ts/,
      use: { baseURL: previewBaseURL }
    },
    {
      name: "questions-trial-real",
      testMatch: /questions-trial\.spec\.ts/,
      use: { baseURL: devBaseURL }
    }
  ],
  webServer: [
    {
      command: `npm run dev -- --host 127.0.0.1 --port ${devPort} --strictPort`,
      url: devBaseURL,
      reuseExistingServer: false,
      stdout: "pipe",
      stderr: "pipe",
      timeout: 120_000
    },
    {
      command:
        `npm run build && npm run preview -- --host 127.0.0.1 --port ${previewPort} --strictPort`,
      url: previewBaseURL,
      reuseExistingServer: false,
      stdout: "pipe",
      stderr: "pipe",
      timeout: 120_000
    },
    {
      command:
        `VESTIBULE_TRIAL_E2E_ADDRESS=127.0.0.1:${trialPort} go test ./internal/console/api -run '^TestQuestionTrialBrowserServer$' -count=1 -timeout=0`,
      cwd: "..",
      url: `${trialBaseURL}/livez`,
      reuseExistingServer: false,
      stdout: "pipe",
      stderr: "pipe",
      timeout: 120_000
    }
  ]
});
