import { spawn, type ChildProcess } from "node:child_process";
import { fileURLToPath } from "node:url";

import { expect, test, type TestInfo } from "@playwright/test";

const repositoryRoot = fileURLToPath(new URL("../..", import.meta.url));
test.use({ ignoreHTTPSErrors: true });

type DailyHarness = Readonly<{
  process: ChildProcess;
  entryURL: string;
}>;

let harness: DailyHarness | null = null;
let runningHarnessProcess: ChildProcess | null = null;

function terminateHarnessProcess(child: ChildProcess): void {
  child.stdin?.end();
  if (!child.pid) {
    return;
  }
  try {
    process.kill(-child.pid, "SIGTERM");
  } catch {
    child.kill("SIGTERM");
  }
}

function stopHarness(): void {
  const child = runningHarnessProcess ?? harness?.process;
  harness = null;
  runningHarnessProcess = null;
  if (child) {
    terminateHarnessProcess(child);
  }
}

function waitForHarness(child: ChildProcess, testInfo: TestInfo): Promise<string> {
  const { promise, resolve, reject } = Promise.withResolvers<string>();
  let settled = false;
  let stdout = "";
  let stderr = "";
  let pending = "";
  const timeout = setTimeout(() => {
    if (!settled) {
      settled = true;
      terminateHarnessProcess(child);
      reject(
        new Error(
          `daily API harness did not become ready within ${testInfo.timeout}ms. stdout: ${stdout}\nstderr: ${stderr}`
        )
      );
    }
  }, 120_000);

  function fail(error: Error): void {
    if (settled) {
      return;
    }
    settled = true;
    clearTimeout(timeout);
    terminateHarnessProcess(child);
    reject(new Error(`${error.message}\nstdout: ${stdout}\nstderr: ${stderr}`));
  }

  child.stdout?.on("data", (chunk: Buffer | string) => {
    stdout += String(chunk);
    pending += String(chunk);
    const lines = pending.split("\n");
    pending = lines.pop() ?? "";
    for (const line of lines) {
      const marker = "DAILY_E2E_READY=";
      const markerIndex = line.indexOf(marker);
      if (markerIndex < 0) {
        continue;
      }
      try {
        const ready: unknown = JSON.parse(line.slice(markerIndex + marker.length));
        if (
          typeof ready !== "object" ||
          ready === null ||
          Array.isArray(ready) ||
          !("entry_url" in ready) ||
          typeof ready.entry_url !== "string" ||
          ready.entry_url.length === 0
        ) {
          fail(new Error("daily API harness emitted an invalid readiness record"));
          return;
        }
        settled = true;
        clearTimeout(timeout);
        resolve(ready.entry_url);
        return;
      } catch (error) {
        fail(new Error(`daily API harness emitted invalid JSON: ${String(error)}`));
        return;
      }
    }
  });
  child.stderr?.on("data", (chunk: Buffer | string) => {
    stderr += String(chunk);
  });
  child.once("error", (error) => fail(error));
  child.once("exit", (code, signal) => {
    if (!settled) {
      fail(new Error(`daily API harness exited before readiness (code=${code}, signal=${signal})`));
    }
  });
  return promise;
}

async function startHarness(testInfo: TestInfo): Promise<string> {
  const override = process.env.DAILY_E2E_ENTRY_URL;
  if (override) {
    return override;
  }

  const frontendURL = testInfo.project.use.baseURL;
  if (typeof frontendURL !== "string" || frontendURL.length === 0) {
    throw new Error("Playwright project must provide a baseURL for the daily API harness");
  }
  const child = spawn(
    "go",
    ["test", "-run", "^TestDailyAPIHarness$", "-count=1", "-v", "./internal/console/api"],
    {
      cwd: repositoryRoot,
      detached: true,
      env: {
        ...process.env,
        RUN_DAILY_API_HARNESS: "1",
        DAILY_E2E_FRONTEND_URL: frontendURL
      },
      stdio: ["pipe", "pipe", "pipe"]
    }
  );
  runningHarnessProcess = child;
  const entryURL = await waitForHarness(child, testInfo);
  harness = { process: child, entryURL };
  return entryURL;
}

test.beforeAll(async ({}, testInfo) => {
  test.setTimeout(150_000);
  const entryURL = await startHarness(testInfo);
  if (!harness && !process.env.DAILY_E2E_ENTRY_URL) {
    throw new Error("daily API harness did not remain available after readiness");
  }
  if (process.env.DAILY_E2E_ENTRY_URL) {
    harness = null;
  }
  testInfo.attachments.push({ name: "daily-api-entry-url", contentType: "text/plain", body: Buffer.from(entryURL) });
});

test.afterAll(() => {
  stopHarness();
});

test("daily status switch persists off and on through the real Go API", async ({ page }) => {
  test.setTimeout(150_000);
  const entryURL = harness?.entryURL ?? process.env.DAILY_E2E_ENTRY_URL;
  if (!entryURL) {
    throw new Error("daily API entry URL is unavailable");
  }

  await page.goto(entryURL);
  await page.goto(new URL("/diagnostics", page.url()).toString());

  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  const daily = screen.locator('[data-diagnostics-section="daily"]');
  const toggle = daily.getByRole("switch");
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await expect(daily).toContainText("09:00");
  const timezone = await page.evaluate(async () => {
    const payload: unknown = await fetch("/api/status/daily").then((response) => response.json());
    if (
      typeof payload !== "object" ||
      payload === null ||
      Array.isArray(payload) ||
      !("timezone" in payload) ||
      typeof payload.timezone !== "string" ||
      payload.timezone.length === 0
    ) {
      throw new Error("daily API returned no effective timezone");
    }
    return payload.timezone;
  });
  await expect(daily).toContainText(timezone);

  await toggle.click();
  await expect(screen.locator('[data-diagnostics-daily-feedback="saved"]')).toBeVisible();
  await expect(screen.locator('[data-diagnostics-daily-feedback="error"]')).toHaveCount(0);
  await expect(toggle).toHaveAttribute("aria-checked", "false");

  await page.reload();
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  await expect(screen.locator("[data-diagnostics-daily-toggle]")).toHaveAttribute(
    "aria-checked",
    "false"
  );
  await screen.locator("[data-diagnostics-daily-toggle]").click();
  await expect(screen.locator('[data-diagnostics-daily-feedback="saved"]')).toBeVisible();
  await expect(screen.locator('[data-diagnostics-daily-feedback="error"]')).toHaveCount(0);
  await expect(screen.locator("[data-diagnostics-daily-toggle]")).toHaveAttribute(
    "aria-checked",
    "true"
  );
  await page.reload();
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  await expect(screen.locator('[data-diagnostics-daily-toggle]')).toHaveAttribute("aria-checked", "true");
  await page.setViewportSize({ width: 320, height: 740 });
  const description = daily.locator("#diagnostics-daily-enabled-description");
  await expect.poll(async () => {
    const copy = await description.boundingBox();
    const card = await daily.boundingBox();
    return copy && card ? copy.width / card.width : 0;
  }).toBeGreaterThan(0.5);
});
