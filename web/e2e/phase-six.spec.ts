import { expect, test, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

const selectedGroupId = "-1001163306055";

const managerSessionPayload = {
  subject: { telegram_id: "741928306", role: "manager" },
  expires_at: "2026-09-01T02:00:00Z",
  csrf_token: "manager-csrf"
} as const;

const groupListErrorCases = [
  {
    code: "authentication_expired",
    status: 401,
    heading: "会话已过期",
    retryable: false
  },
  {
    code: "authentication_invalid",
    status: 401,
    heading: "无法验证会话",
    retryable: false
  },
  {
    code: "authentication_unavailable",
    status: 503,
    heading: "认证服务暂时不可用",
    retryable: true
  },
  {
    code: "verification_unavailable",
    status: 503,
    heading: "群列表暂时不可用",
    retryable: true
  }
] as const;
const pendingQueueEntry = {
  id: "-1001163306055:528106774:queue-nonce",
  user: "@another",
  group_key: selectedGroupId,
  result: { state: "pending", reason: null },
  occurred_at: "2026-08-31T14:09:00+08:00",
  expires_at: "2026-08-31T14:11:41+08:00",
  remaining_seconds: 161
};

const approvedQueueEntry = {
  ...pendingQueueEntry,
  result: { state: "approved", reason: null },
  remaining_seconds: null
};

const homeSettingsPayload = {
  enabled: { value: true, source: "factory default" },
  delivery_mode: { value: "both", source: "factory default" },
  verify_mode: { value: "mixed", source: "factory default" },
  timeout_seconds: { value: 300, source: "factory default" },
  questions: { value: [], source: "factory default" },
  fallback_questions: { value: [], source: "factory default" },
  trusted_member_group_ids: { value: [], source: "factory default" },
  channel_whitelist: { value: [], source: "factory default" },
  antispam_enabled: { value: true, source: "factory default" },
  warn_limit: { value: 3, source: "factory default" }
} as const;

const healthyDiagnosticsPayload = {
  health: {
    live: true,
    ready: true,
    config_ready: true,
    telegram_ready: true
  },
  bot_api: {
    last_heartbeat_at: "2026-09-01T01:00:00Z",
    latency_ms: 12
  },
  persistence: {
    configured: true,
    durable: true,
    writable: true,
    last_error: null
  }
} as const;

function homeStatsPayload(url: URL): unknown {
  return {
    range: {
      from: url.searchParams.get("from"),
      to: url.searchParams.get("to"),
      timezone: url.searchParams.get("timezone")
    },
    summary: {
      challenges: 1,
      approved: 1,
      declined: 0,
      banned: 0,
      expired: 0,
      pass_rate: 1
    },
    trend: [],
    interceptions: []
  };
}

type QueueReadHandler = (route: Route, readCount: number) => Promise<void>;
type SettlementHandler = (route: Route) => Promise<void>;

function rowFor(page: Page, user: string) {
  return page.locator("[data-queue-row]", { hasText: user });
}

async function mockQueueTransport(
  page: Page,
  readQueue: QueueReadHandler,
  settleQueue: SettlementHandler
): Promise<void> {
  let readCount = 0;

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: { telegram_id: "741928306", role: "manager" },
          expires_at: "2026-09-01T02:00:00Z",
          csrf_token: "manager-csrf"
        })
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupId }] })
      });
      return;
    }

    if (path === `/api/chats/${selectedGroupId}/queue` && request.method() === "GET") {
      await readQueue(route, readCount);
      readCount += 1;
      return;
    }

    if (
      path === `/api/chats/${selectedGroupId}/queue/${pendingQueueEntry.id}` &&
      request.method() === "POST"
    ) {
      await settleQueue(route);
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openFixtureQueue(page: Page): Promise<void> {
  await page.goto("/queue");
  await expect(page.locator("[data-queue-page]")).toHaveAttribute(
    "data-queue-state",
    "populated"
  );
}

async function openLiveQueue(
  page: Page,
  settleQueue: SettlementHandler,
  readQueue: QueueReadHandler = async (route) => {
    await route.fulfill({
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ items: [pendingQueueEntry] })
    });
  }
): Promise<void> {
  await mockQueueTransport(page, readQueue, settleQueue);
  await page.goto(`/queue?group=${selectedGroupId}`);
  await expect(page.locator("[data-queue-page]")).toHaveAttribute(
    "data-queue-state",
    "populated"
  );
}

test("Mini App session exchange reaches a successful release", async ({ page }) => {
  const initData = "query_id=telegram-test&user=%7B%22id%22%3A741928306%7D&hash=signature";
  const sessionRequests: string[] = [];

  await page.addInitScript((data) => {
    Object.defineProperty(window, "Telegram", {
      configurable: true,
      value: { WebApp: { initData: data } }
    });
  }, initData);
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = decodeURIComponent(url.pathname);
    sessionRequests.push(`${request.method()} ${path}`);

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        status: 401,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ error: { code: "authentication_expired" } })
      });
      return;
    }

    if (path === "/api/session" && request.method() === "POST") {
      expect(JSON.parse(request.postData() ?? "")).toEqual({ init_data: initData });
      expect(request.headers()["x-csrf-token"]).toBeUndefined();
      await route.fulfill({
        status: 201,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: { telegram_id: "741928306", role: "manager" },
          expires_at: "2026-09-01T02:00:00Z",
          csrf_token: "mini-app-csrf"
        })
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupId }] })
      });
      return;
    }

    if (path === `/api/chats/${selectedGroupId}/queue` && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [pendingQueueEntry] })
      });
      return;
    }

    if (path === `/api/chats/${selectedGroupId}/stats` && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(homeStatsPayload(url))
      });
      return;
    }

    if (path === `/api/chats/${selectedGroupId}/settings` && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(homeSettingsPayload)
      });
      return;
    }

    if (
      path === `/api/chats/${selectedGroupId}/queue/${pendingQueueEntry.id}` &&
      request.method() === "POST"
    ) {
      expect(request.headers()["x-csrf-token"]).toBe("mini-app-csrf");
      expect(JSON.parse(request.postData() ?? "")).toEqual({
        expected: { state: "pending", reason: "" },
        result: { state: "approved", reason: "" }
      });
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(approvedQueueEntry)
      });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await test.step("exchange the Mini App identity for a session", async () => {
    await page.goto("/");
    await expect(page).toHaveURL(new RegExp(`/home\\?group=${selectedGroupId}$`));
    await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
    expect(sessionRequests.slice(0, 3)).toEqual([
      "GET /api/session",
      "POST /api/session",
      "GET /api/chats"
    ]);
  });

  await test.step("review the selected managed group", async () => {
    // Home already selects the first authorised group, so this step reads the
    // switcher rather than operating it. The switcher is no longer a native
    // select, so its value is an attribute, not a form value.
    const groupSwitcher = page.getByRole("button", { name: "当前群" });
    await expect(groupSwitcher).toHaveAttribute("data-value", selectedGroupId);
    await page.getByRole("link", { name: "群与频道", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/groups\\?group=${selectedGroupId}$`));

    const selectedRow = page.locator("[data-group-row][data-selected]");
    await expect(selectedRow).toContainText(selectedGroupId);
    await expect(selectedRow).not.toContainText("Gentoo 中文社区");
  });

  await test.step("open the waiting queue", async () => {
    await page.getByRole("link", { name: "等待队列", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/queue\\?group=${selectedGroupId}$`));
    await expect(page.locator("[data-queue-page]")).toHaveAttribute(
      "data-queue-state",
      "populated"
    );
  });

  await test.step("release one waiting applicant", async () => {
    const row = rowFor(page, "@another");
    await expect(row).toHaveAttribute("data-result", "pending");
    await expect(row.locator("[data-queue-result]")).toContainText("等待中 2:41");

    await row.getByRole("button", { name: "放行 @another" }).click();

    await expect(row).toHaveAttribute("data-action-state", "idle");
    await expect(row).toHaveAttribute("data-result", "approved");
    await expect(row.locator("[data-queue-result]")).toHaveText("已通过");
    await expect(page.locator("[data-queue-feedback]")).toContainText("已放行 @another");
  });
});

test("operator cookie session skips Mini App exchange", async ({ page, baseURL }) => {
  const requests: string[] = [];

  await page.context().addCookies([
    {
      name: "vestibule_console_session",
      value: "operator-session",
      url: baseURL ?? "http://127.0.0.1:4173"
    }
  ]);
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    requests.push(`${request.method()} ${path}`);

    if (path === "/api/session" && request.method() === "GET") {
      expect(request.headers().cookie ?? "").toContain(
        "vestibule_console_session=operator-session"
      );
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: { telegram_id: "741928306", role: "operator" },
          expires_at: "2026-09-01T02:00:00Z",
          csrf_token: "operator-csrf"
        })
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupId }] })
      });
      return;
    }

    if (path === `/api/chats/${selectedGroupId}/queue` && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [] })
      });
      return;
    }

    if (path === `/api/chats/${selectedGroupId}/stats` && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(homeStatsPayload(url))
      });
      return;
    }

    if (path === `/api/chats/${selectedGroupId}/settings` && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(homeSettingsPayload)
      });
      return;
    }

    if (path === "/api/status" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(healthyDiagnosticsPayload)
      });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await page.goto("/");
  await expect(page).toHaveURL(new RegExp(`/home\\?group=${selectedGroupId}$`));
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
  expect(requests.slice(0, 2)).toEqual(["GET /api/session", "GET /api/chats"]);
  expect(requests).not.toContain("POST /api/session");
});

test("a valid session with no groups identifies the Telegram account", async ({ page }) => {
  const requests: string[] = [];

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    requests.push(`${request.method()} ${path}`);

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          subject: { telegram_id: "741928306", role: "manager" },
          expires_at: "2026-09-01T02:00:00Z",
          csrf_token: "manager-csrf"
        })
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [] })
      });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await page.goto("/");
  const entry = page.locator("[data-entry-page]");
  await expect(entry).toHaveAttribute("data-entry-state", "no-groups");
  await expect(
    page.getByRole("heading", { name: "这个账号尚未获任何群授权" })
  ).toBeVisible();
  await expect(entry).toContainText("741928306");
  expect(requests).toEqual(["GET /api/session", "GET /api/chats"]);
});

test("group list keeps loading distinct from a settled empty result", async ({ page }) => {
  let releaseChats: (() => void) | undefined;
  const chatsReleased = new Promise<void>((resolve) => {
    releaseChats = resolve;
  });

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(managerSessionPayload)
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await chatsReleased;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [] })
      });
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await page.goto("/groups");
  const groups = page.locator("[data-groups-page]");
  await expect(groups).toHaveAttribute("data-groups-state", "loading");
  await expect(page.getByRole("heading", { name: "没有可管理的群" })).toHaveCount(0);

  expect(releaseChats).toBeDefined();
  releaseChats?.();
  await expect(groups).toHaveAttribute("data-groups-state", "empty");
  await expect(page.getByRole("heading", { name: "没有可管理的群" })).toBeVisible();
  await expect(groups).toContainText("741928306");
  await expect(page.getByRole("button", { name: "重新读取" })).toHaveCount(0);
});

test("group-list error matrix covers every explicit backend code", () => {
  expect(groupListErrorCases.map(({ code }) => code)).toEqual([
    "authentication_expired",
    "authentication_invalid",
    "authentication_unavailable",
    "verification_unavailable"
  ]);
});

for (const errorCase of groupListErrorCases) {
  test(`group list presents ${errorCase.code} and its recovery`, async ({ page }) => {
    let chatRequestCount = 0;

    await page.route("**/api/**", async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname;

      if (path === "/api/session" && request.method() === "GET") {
        await route.fulfill({
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(managerSessionPayload)
        });
        return;
      }
      if (path === "/api/chats" && request.method() === "GET") {
        chatRequestCount += 1;
        await route.fulfill(
          chatRequestCount === 1
            ? {
                status: errorCase.status,
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ error: { code: errorCase.code } })
              }
            : {
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ chats: [{ id: selectedGroupId }] })
              }
        );
        return;
      }
      throw new Error(`Unexpected API request: ${request.method()} ${path}`);
    });

    await page.goto("/groups");
    const groups = page.locator("[data-groups-page]");
    const error = page.locator("[data-group-state='error']");
    await expect(groups).toHaveAttribute("data-groups-state", "error");
    await expect(error).toHaveAttribute("data-group-error-code", errorCase.code);
    await expect(page.getByRole("heading", { name: errorCase.heading })).toBeVisible();

    if (errorCase.retryable) {
      await page.getByRole("button", { name: "重新读取" }).click();
      await expect(groups).toHaveAttribute("data-groups-state", "populated");
      await expect(page.locator("[data-group-row]")).toContainText(selectedGroupId);
      expect(chatRequestCount).toBe(2);
    } else {
      await expect(page.getByRole("button", { name: "重新读取" })).toHaveCount(0);
      expect(chatRequestCount).toBe(1);
    }
  });
}

test("group list retains its fixture fallback without an API", async ({ page }) => {
  await page.goto("/groups");

  const groups = page.locator("[data-groups-page]");
  await expect(groups).toHaveAttribute("data-groups-source", "fixtures");
  await expect(groups).toHaveAttribute("data-groups-state", "populated");
  await expect(page.locator("[data-group-row]")).toHaveCount(3);
  await expect(
    page.getByRole("link", { name: `查看群 ${selectedGroupId} 的等待队列` })
  ).toBeVisible();
});

test("keyboard selection carries the group boundary to the queue", async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(managerSessionPayload)
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupId }] })
      });
      return;
    }
    // Pressing Enter lands on /queue, which fetches. Whether that request
    // arrives before this test ends is a race, and without this branch the
    // handler below turns a slow machine into a failure.
    if (path === `/api/chats/${selectedGroupId}/queue` && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [] })
      });
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await page.goto("/groups");
  await expect(page.locator("[data-groups-page]")).toHaveAttribute(
    "data-groups-source",
    "api"
  );
  expect(await page.evaluate(() => document.activeElement?.tagName)).toBe("BODY");

  // The group row sits after everything before it in the tab order, and what
  // comes before it grows with the console: twelve presses reached it at three
  // screens and stopped reaching it at eight. Counting the focusable elements
  // on the page makes the bound follow the page instead of a number that was
  // true once.
  const focusableCount = await page.evaluate(
    () =>
      document.querySelectorAll(
        'a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])'
      ).length
  );
  let focusedGroupId: string | null = null;
  for (let index = 0; index < focusableCount + 2; index += 1) {
    await page.keyboard.press("Tab");
    focusedGroupId = await page.evaluate(
      () => document.activeElement?.getAttribute("data-select-group") ?? null
    );
    if (focusedGroupId === selectedGroupId) {
      break;
    }
  }

  expect(focusedGroupId).toBe(selectedGroupId);
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(new RegExp(`/queue\\?group=${selectedGroupId}$`));
  // Wait for the queue to settle, not just for the URL. The URL changes before
  // the queue fetch goes out, so ending here leaves that request racing the end
  // of the test — it arrived on CI and not on this machine, and the handler
  // above turned it into a failure.
  await expect(page.locator("[data-queue-page]")).toHaveAttribute(
    "data-queue-state",
    "empty"
  );
});

test("widest locale keeps group controls inside the desktop header", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/session" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(managerSessionPayload)
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ chats: [{ id: selectedGroupId }] })
      });
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await page.goto("/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();
  await selectAppOption(page.getByRole("button", { name: "语言" }), "en");
  await expect(page.locator("html")).toHaveAttribute("lang", "en");

  const bounds = await page.evaluate(() => {
    const header = document.querySelector("[data-console-header]");
    const controls = document.querySelector("[data-header-controls]");
    if (!(header instanceof HTMLElement) || !(controls instanceof HTMLElement)) {
      throw new Error("Group header geometry targets are missing");
    }
    const headerRect = header.getBoundingClientRect();
    const controlsRect = controls.getBoundingClientRect();
    return {
      headerTop: headerRect.top,
      headerBottom: headerRect.bottom,
      controlsTop: controlsRect.top,
      controlsBottom: controlsRect.bottom
    };
  });

  expect(bounds.controlsTop).toBeGreaterThanOrEqual(bounds.headerTop);
  expect(bounds.controlsBottom).toBeLessThanOrEqual(bounds.headerBottom);
});

test("entry retains its fixture fallback without an API", async ({ page }) => {
  await page.goto("/");

  const entry = page.locator("[data-entry-page]");
  await expect(entry).toHaveAttribute("data-entry-state", "no-session");
  await expect(entry).toHaveAttribute("data-entry-transport-failure", "non-json");
  await expect(
    page.getByRole("heading", { name: "从 Telegram 打开控制台" })
  ).toBeVisible();
});

test("entry states retain their distinct guidance", async ({ page }) => {
  const stateCases = [
    ["no-session", "从 Telegram 打开控制台"],
    ["expired", "这条链接已过期"],
    ["redeemed", "这条链接已被用过"],
    ["outside-telegram", "群管理仅可从 Telegram 内打开"]
  ] as const;
  expect(stateCases).not.toHaveLength(0);

  await page.route("**/api/session", async (route) => {
    await route.fulfill({
      status: 401,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ error: { code: "authentication_expired" } })
    });
  });

  for (const [state, heading] of stateCases) {
    await page.goto(`/?state=${state}`);
    await expect(page.locator("[data-entry-page]")).toHaveAttribute("data-entry-state", state);
    await expect(page.getByRole("heading", { name: heading })).toBeVisible();
  }
});

test("queue waits for a real response before rendering empty", async ({ page }) => {
  let markQueueRequested!: () => void;
  let resolveQueue!: () => void;
  const queueRequested = new Promise<void>((resolve) => {
    markQueueRequested = resolve;
  });
  const queueResponse = new Promise<void>((resolve) => {
    resolveQueue = resolve;
  });

  await mockQueueTransport(
    page,
    async (route) => {
      markQueueRequested();
      await queueResponse;
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ items: [] })
      });
    },
    async () => {
      throw new Error("Settlement must not run while loading the queue");
    }
  );

  await page.goto(`/queue?group=${selectedGroupId}`, { waitUntil: "domcontentloaded" });
  await expect(page.locator("[data-queue-page]")).toHaveAttribute("data-queue-state", "loading");
  await expect(page.getByRole("heading", { name: "还没有人申请" })).toHaveCount(0);
  await queueRequested;
  resolveQueue();

  await expect(page.locator("[data-queue-page]")).toHaveAttribute("data-queue-state", "empty");
  await expect(page.getByRole("heading", { name: "还没有人申请" })).toBeVisible();
});

test("queue uses the API code for an unavailable load", async ({ page }) => {
  await mockQueueTransport(
    page,
    async (route) => {
      await route.fulfill({
        status: 503,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ error: { code: "chat_access_denied" } })
      });
    },
    async () => {
      throw new Error("Settlement must not run when the queue is unavailable");
    }
  );

  await page.goto(`/queue?group=${selectedGroupId}`);

  await expect(page.locator("[data-queue-page]")).toHaveAttribute(
    "data-queue-state",
    "unavailable"
  );
  await expect(page.locator("[data-queue-unavailable]")).toContainText(
    "你已不再拥有此群的管理权限"
  );
});

test("a failed fixture release restores pending state and remaining time", async ({ page }) => {
  await openFixtureQueue(page);

  const row = rowFor(page, "@retry_release");
  const result = row.locator("[data-queue-result]");
  await expect(row).toHaveAttribute("data-result", "pending");
  await expect(result).toContainText("等待中 3:39");

  await row.getByRole("button", { name: "放行 @retry_release" }).click();
  await expect(row).toHaveAttribute("data-result", "approved");

  await expect(page.locator("[data-queue-feedback]")).toContainText("未能放行 @retry_release");
  await expect(row).toHaveAttribute("data-action-state", "idle");
  await expect(row).toHaveAttribute("data-result", "pending");
  await expect(result).toContainText("等待中 3:39");
});

test("banned fixture rows expose status without an action", async ({ page }) => {
  await openFixtureQueue(page);

  const row = rowFor(page, "@spam_ad_01");
  await expect(row).toHaveAttribute("data-result", "banned");
  await expect(row.getByRole("button")).toHaveCount(0);
});

test("narrow queue exposes a release card to keyboard users", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await openLiveQueue(page, async (route) => {
    await route.fulfill({
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(approvedQueueEntry)
    });
  });

  await expect(page.locator("[data-queue-table-scroll]")).toBeHidden();

  const card = page.locator("[data-queue-card-row]", { hasText: "@another" });
  const action = card.getByRole("button", { name: "放行 @another" });
  await expect(card).toHaveAttribute("data-result", "pending");
  await expect(action).toBeVisible();

  await action.focus();
  await expect(action).toBeFocused();
  await page.keyboard.press("Enter");

  await expect(card).toHaveAttribute("data-result", "approved");
  await expect(card.locator("[data-queue-action-id=\"release\"]")).toHaveCount(0);
});

const settlementFailureCases = [
  {
    name: "loss of management permission",
    kind: "api",
    code: "chat_access_denied",
    message: "你已不再拥有此群的管理权限",
    expectedState: "unavailable"
  },
  {
    name: "a concurrent settlement",
    kind: "api",
    code: "challenge_conflict",
    message: "此申请已由其他管理员处理",
    expectedState: "empty"
  },
  {
    name: "an expired CSRF token",
    kind: "api",
    code: "csrf_invalid",
    message: "此页的安全令牌已过期",
    expectedState: "populated"
  },
  {
    name: "an interrupted network request",
    kind: "network",
    message: "连接已中断",
    expectedState: "populated"
  }
] as const;

for (const failure of settlementFailureCases) {
  test(`release distinguishes ${failure.name}`, async ({ page }) => {
    let queueReadCount = 0;
    await openLiveQueue(
      page,
      async (route) => {
        if (failure.kind === "network") {
          await route.abort("failed");
          return;
        }

        await route.fulfill({
          status: 503,
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ error: { code: failure.code } })
        });
      },
      async (route) => {
        queueReadCount += 1;
        await route.fulfill({
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            items:
              failure.kind === "api" &&
              failure.code === "challenge_conflict" &&
              queueReadCount > 1
                ? []
                : [pendingQueueEntry]
          })
        });
      }
    );

    const row = rowFor(page, "@another");
    await row.getByRole("button", { name: "放行 @another" }).click();

    await expect(page.locator("[data-queue-feedback]")).toContainText(failure.message);
    await expect(page.locator("[data-queue-page]")).toHaveAttribute(
      "data-queue-state",
      failure.expectedState
    );

    if (failure.kind === "api" && failure.code === "challenge_conflict") {
      await expect(row).toHaveCount(0);
      expect(queueReadCount).toBe(2);
      return;
    }

    if (failure.expectedState === "unavailable") {
      await expect(row).toHaveCount(0);
      return;
    }

    await expect(row).toHaveAttribute("data-result", "pending");
    await expect(row).toHaveAttribute("data-action-state", "idle");
  });
}
