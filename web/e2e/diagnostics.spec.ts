import { expect, test, type Page, type Route } from "@playwright/test";

const actorID = "9000000301";
const selectedGroupID = "-1009000000302";

type ConsoleRole = "manager" | "operator";
type DiagnosticsHandler = (route: Route) => Promise<void>;

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockDiagnosticsTransport(
  page: Page,
  role: ConsoleRole,
  diagnostics: DiagnosticsHandler
): Promise<string[]> {
  const statusMethods: string[] = [];
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role },
        expires_at: "2026-09-02T12:00:00Z",
        csrf_token: "diagnostics-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID }] });
      return;
    }
    if (path === "/api/status") {
      statusMethods.push(request.method());
      await diagnostics(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
  return statusMethods;
}

const unmeasuredDiagnostics = {
  health: {
    live: true,
    ready: false,
    config_ready: true,
    telegram_ready: false
  },
  bot_api: {
    last_heartbeat_at: null,
    latency_ms: 0
  },
  persistence: {
    configured: true,
    durable: true,
    writable: true,
    last_error: null
  }
} as const;

test("diagnostics preserves null probes separately from a measured zero latency", async ({ page }) => {
  const statusMethods = await mockDiagnosticsTransport(page, "operator", async (route) => {
    if (route.request().method() !== "GET") {
      throw new Error(`Unexpected diagnostics write: ${route.request().method()}`);
    }
    await fulfillJSON(route, unmeasuredDiagnostics);
  });

  await page.goto(`/diagnostics?group=${selectedGroupID}`);
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  await expect(screen.locator("input, textarea, select, button")).toHaveCount(0);
  expect(statusMethods).toEqual(["GET"]);

  await expect(screen.locator('[data-diagnostics-value="live"]')).toContainText("是");
  await expect(screen.locator('[data-diagnostics-value="ready"]')).toContainText("否");
  const heartbeat = screen.locator('[data-diagnostics-value="last-heartbeat"]');
  await expect(heartbeat).toContainText("尚未测量");
  await expect(heartbeat.locator("time")).toHaveCount(0);
  const latency = screen.locator('[data-diagnostics-value="latency"]');
  await expect(latency).toContainText("0 毫秒");
  await expect(latency.getByText("尚未测量", { exact: true })).toHaveCount(0);
  await expect(screen.getByText("1970", { exact: false })).toHaveCount(0);
  await expect(screen.locator('[data-diagnostics-value="last-error"]')).toContainText(
    "未记录失败原因"
  );

  await expect(screen.locator("[data-diagnostics-unreported-item]")).toHaveCount(3);
  await expect(screen.locator('[data-diagnostics-unreported-item="permission-preflight"]')).toContainText(
    "不能以 Telegram 就绪代替"
  );
  await expect(screen.locator('[data-diagnostics-unreported-item="query-cache"]')).toContainText(
    "没有缓存状态或命中数据"
  );
  await expect(screen.locator('[data-diagnostics-unreported-item="query-rate-limit"]')).toContainText(
    "按群设置"
  );
});

test("diagnostics tells group managers that instance state is not a load failure", async ({ page }) => {
  const statusMethods = await mockDiagnosticsTransport(page, "manager", async (route) => {
    await fulfillJSON(route, { error: { code: "diagnostics_access_denied" } }, 403);
  });

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "access-denied");
  await expect(
    screen.getByRole("heading", { name: "这是实例状态，不归群管理员查看" })
  ).toBeVisible();
  await expect(screen).toContainText("此屏仅向运维开放");
  await expect(screen.getByText("无法读取实例状态", { exact: true })).toHaveCount(0);
  await expect(screen.getByRole("button", { name: "重试" })).toHaveCount(0);
  expect(statusMethods).toEqual(["GET"]);
});

test("diagnostics renders measured Bot API data and the recorded persistence failure", async ({ page }) => {
  const statusMethods = await mockDiagnosticsTransport(page, "operator", async (route) => {
    if (route.request().method() !== "GET") {
      throw new Error(`Unexpected diagnostics write: ${route.request().method()}`);
    }
    await fulfillJSON(route, {
      health: {
        live: true,
        ready: true,
        config_ready: true,
        telegram_ready: true
      },
      bot_api: {
        last_heartbeat_at: "2026-09-02T08:15:30Z",
        latency_ms: 27
      },
      persistence: {
        configured: true,
        durable: false,
        writable: false,
        last_error: "settings database is read-only"
      }
    });
  });

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  await expect(
    screen.locator('[data-diagnostics-value="last-heartbeat"] time')
  ).toHaveAttribute("dateTime", "2026-09-02T08:15:30Z");
  await expect(screen.locator('[data-diagnostics-value="latency"]')).toContainText("27 毫秒");
  await expect(screen.locator('[data-diagnostics-value="durable"]')).toContainText("否");
  await expect(screen.locator('[data-diagnostics-value="writable"]')).toContainText("否");
  await expect(screen.locator('[data-diagnostics-value="last-error"] code')).toHaveText(
    "settings database is read-only"
  );
  expect(statusMethods).toEqual(["GET"]);
});

test("diagnostics retries an unavailable read through the component button", async ({ page }) => {
  let attempts = 0;
  const statusMethods = await mockDiagnosticsTransport(page, "operator", async (route) => {
    if (route.request().method() !== "GET") {
      throw new Error(`Unexpected diagnostics write: ${route.request().method()}`);
    }
    attempts += 1;
    if (attempts === 1) {
      await fulfillJSON(route, { error: { code: "diagnostics_unavailable" } }, 503);
      return;
    }
    await fulfillJSON(route, unmeasuredDiagnostics);
  });

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "unavailable");
  const retry = screen.getByRole("button", { name: "重试" });
  await expect(retry).toHaveAttribute("data-slot", "button");

  await retry.click();

  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  await expect(screen.locator("input, textarea, select, button")).toHaveCount(0);
  expect(statusMethods).toEqual(["GET", "GET"]);
});
