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

type StreakOverrides = Readonly<{
  first_problem_at?: string | null;
  last_problem_at?: string | null;
  last_recovered_at?: string | null;
  problem_span_seconds?: number;
  exceeds_threshold?: boolean;
}>;

function streak(overrides: StreakOverrides = {}) {
  return {
    threshold_seconds: 600,
    first_problem_at: null,
    last_problem_at: null,
    last_recovered_at: null,
    problem_events: 0,
    problem_span_seconds: 0,
    exceeds_threshold: false,
    ...overrides
  };
}

type RollbackOverrides = Readonly<{
  rejections?: Record<string, unknown>;
  challenge_delivery?: Record<string, unknown>;
  console_access?: Record<string, unknown>;
  database_writes?: Record<string, unknown>;
}>;

function rollbackObservations(overrides: RollbackOverrides = {}) {
  return {
    rejections: {
      source_available: true,
      human_review_required: true,
      window_seconds: 86400,
      window_start: "2026-09-01T08:00:00Z",
      window_end: "2026-09-02T08:00:00Z",
      by_reason: [],
      ...overrides.rejections
    },
    challenge_delivery: {
      streak: streak(),
      failed_deliveries: 0,
      duplicate_deliveries: 0,
      ...overrides.challenge_delivery
    },
    console_access: {
      streak: streak(),
      unavailable_attempts: 0,
      ...overrides.console_access
    },
    database_writes: {
      scope: "retry_store_write",
      window_seconds: 600,
      window_start: "2026-09-02T07:50:00Z",
      window_end: "2026-09-02T08:00:00Z",
      total_writes: 0,
      failed_writes: 0,
      failure_rate_percent: 0,
      exceeds_one_percent: false,
      ...overrides.database_writes
    }
  };
}

function diagnosticsWithRollback(rollback: ReturnType<typeof rollbackObservations>) {
  return {
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
      durable: true,
      writable: true,
      last_error: null
    },
    rollback_observations: rollback
  };
}

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
    screen.getByRole("heading", { name: "群管理员无权查看实例状态" })
  ).toBeVisible();
  await expect(screen).toContainText("此页面仅供运维人员使用");
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

test("diagnostics reads a single delivery failure and a sustained outage differently", async ({
  page
}) => {
  const statusMethods = await mockDiagnosticsTransport(page, "operator", async (route) => {
    await fulfillJSON(
      route,
      diagnosticsWithRollback(
        rollbackObservations({
          challenge_delivery: {
            streak: streak({
              first_problem_at: "2026-09-02T08:14:00Z",
              last_problem_at: "2026-09-02T08:14:00Z",
              problem_span_seconds: 0
            }),
            failed_deliveries: 1,
            duplicate_deliveries: 0
          },
          console_access: {
            streak: streak({
              first_problem_at: "2026-09-02T08:01:26Z",
              last_problem_at: "2026-09-02T08:14:00Z",
              last_recovered_at: "2026-09-02T07:40:00Z",
              problem_span_seconds: 754.2,
              exceeds_threshold: true
            }),
            unavailable_attempts: 9
          }
        })
      )
    );
  });

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  await expect(screen.locator("input, textarea, select, button")).toHaveCount(0);
  expect(statusMethods).toEqual(["GET"]);

  const delivery = screen.locator('[data-diagnostics-rollback-item="challenge-delivery"]');
  await expect(delivery).toHaveAttribute("data-diagnostics-rollback-state", "within");
  await expect(delivery).toContainText("未达阈值");
  await expect(delivery).toContainText("阈值 10 分钟");
  await expect(delivery).toContainText("单次失败的连续时长为 0");
  await expect(delivery.locator('[data-diagnostics-value="challenge-delivery-span"]')).toContainText(
    "0 秒"
  );
  await expect(
    delivery.locator('[data-diagnostics-value="challenge-delivery-failed"]')
  ).toContainText("1");
  await expect(delivery.locator('[data-status="error"]')).toHaveCount(0);

  const access = screen.locator('[data-diagnostics-rollback-item="console-access"]');
  await expect(access).toHaveAttribute("data-diagnostics-rollback-state", "exceeded");
  await expect(access).toContainText("已超过阈值");
  await expect(access.locator('[data-diagnostics-value="console-access-span"]')).toContainText(
    "12 分 34 秒"
  );
  await expect(access.locator('[data-status="error"]')).toHaveCount(1);
  await expect(access).not.toContainText("未达阈值");
});

test("diagnostics shows the write denominator and leaves declines as raw material", async ({
  page
}) => {
  await mockDiagnosticsTransport(page, "operator", async (route) => {
    await fulfillJSON(
      route,
      diagnosticsWithRollback(
        rollbackObservations({
          database_writes: {
            total_writes: 3000,
            failed_writes: 12,
            failure_rate_percent: 0.4,
            exceeds_one_percent: false
          },
          rejections: {
            by_reason: [
              { reason: "challenge_timeout", count: 3 },
              { reason: null, count: 1 }
            ]
          }
        })
      )
    );
  });

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");

  const writes = screen.locator('[data-diagnostics-rollback-item="database-writes"]');
  await expect(writes).toHaveAttribute("data-diagnostics-rollback-state", "within");
  await expect(writes.locator('[data-diagnostics-value="database-writes-rate"]')).toContainText(
    "0.4%（12 / 3,000 次写入）"
  );
  await expect(writes).toContainText("观测窗口 10 分钟");
  await expect(writes.locator('[data-diagnostics-value="database-writes-scope"] code')).toHaveText(
    "retry_store_write"
  );

  const rejections = screen.locator('[data-diagnostics-rollback-item="rejections"]');
  await expect(rejections).toHaveAttribute("data-diagnostics-rollback-state", "listed");
  await expect(rejections).toContainText("需人工判读");
  await expect(rejections).toContainText("不代表有人被错误拒绝");
  await expect(rejections).toContainText("最近 24 小时");
  await expect(
    rejections.locator('[data-diagnostics-rejection-reason="challenge_timeout"]')
  ).toContainText("3");
  await expect(rejections).toContainText("未记录原因");
  // A count grouped by reason is not a verdict, so the reading carries no pass
  // or fail badge for anyone to read one out of.
  await expect(rejections.locator('[data-status="ok"], [data-status="error"]')).toHaveCount(0);
});

test("diagnostics does not read an empty write window as a rate inside the limit", async ({
  page
}) => {
  await mockDiagnosticsTransport(page, "operator", async (route) => {
    await fulfillJSON(
      route,
      diagnosticsWithRollback(
        rollbackObservations({ rejections: { source_available: false, by_reason: [] } })
      )
    );
  });

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");

  const writes = screen.locator('[data-diagnostics-rollback-item="database-writes"]');
  await expect(writes).toHaveAttribute("data-diagnostics-rollback-state", "no-writes");
  await expect(writes).toContainText("窗口内没有写入");
  await expect(writes.locator('[data-status="ok"]')).toHaveCount(0);
  await expect(writes).not.toContainText("0%");

  const rejections = screen.locator('[data-diagnostics-rollback-item="rejections"]');
  await expect(rejections).toHaveAttribute("data-diagnostics-rollback-state", "unavailable");
  await expect(rejections).toContainText("本次没有读到拒绝记录");

  const delivery = screen.locator('[data-diagnostics-rollback-item="challenge-delivery"]');
  await expect(delivery).toHaveAttribute("data-diagnostics-rollback-state", "clear");
  await expect(delivery).toContainText("无未恢复的失败");
  await expect(
    delivery.locator('[data-diagnostics-value="challenge-delivery-last-recovered"]')
  ).toContainText("未记录");
});

test("diagnostics keeps working when the instance reports no rollback readings", async ({
  page
}) => {
  await mockDiagnosticsTransport(page, "operator", async (route) => {
    await fulfillJSON(route, unmeasuredDiagnostics);
  });

  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  await expect(screen.locator("[data-diagnostics-rollback-unavailable]")).toContainText(
    "当前实例没有返回回退读数"
  );
  await expect(screen.locator("[data-diagnostics-rollback-item]")).toHaveCount(0);
});

test("a long decline reason scrolls inside its own list at 320px", async ({ page }) => {
  const reason = "challenge_delivery_rejected_by_upstream_bot_api_after_exhausted_retries_1234567890";
  await mockDiagnosticsTransport(page, "operator", async (route) => {
    await fulfillJSON(
      route,
      diagnosticsWithRollback(
        rollbackObservations({ rejections: { by_reason: [{ reason, count: 2 }] } })
      )
    );
  });

  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto("/diagnostics");
  const screen = page.locator("[data-diagnostics-page]");
  await expect(screen).toHaveAttribute("data-diagnostics-state", "loaded");
  await expect(
    screen.locator(`[data-diagnostics-rejection-reason="${reason}"] code`)
  ).toHaveText(reason);

  // A decline reason arrives from the instance at whatever length it has. It may
  // scroll inside its own list; it may not widen the column that holds every
  // other reading. The shell itself scrolls, so measuring the document would
  // never see this.
  const column = await screen.locator("[data-diagnostics-content]").evaluate((element) => ({
    scrollWidth: element.scrollWidth,
    clientWidth: element.clientWidth
  }));
  expect(column.scrollWidth).toBeLessThanOrEqual(column.clientWidth + 1);
});
