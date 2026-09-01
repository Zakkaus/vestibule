import { expect, test, type Page } from "@playwright/test";

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

function rowFor(page: Page, user: string) {
  return page.locator("[data-queue-row]", { hasText: user });
}

async function openFixtureQueue(page: Page): Promise<void> {
  await page.goto("/queue");
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
    const path = new URL(request.url()).pathname;
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

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await test.step("exchange the Mini App identity for a session", async () => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/groups$/);
    await expect(page.locator("[data-groups-page]")).toHaveAttribute(
      "data-groups-source",
      "api"
    );
    expect(sessionRequests).toEqual([
      "GET /api/session",
      "POST /api/session",
      "GET /api/chats"
    ]);
  });

  await test.step("select a managed group", async () => {
    const groupSwitcher = page.getByRole("combobox", { name: "当前群" });
    await groupSwitcher.selectOption(selectedGroupId);
    await expect(groupSwitcher).toHaveValue(selectedGroupId);
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

    await expect(row).toHaveAttribute("data-result", "approved");
    const feedback = page.locator(
      '[data-queue-feedback][data-feedback-kind="releaseSuccess"]'
    );
    await expect(feedback).toContainText("已放行 @another");
    await expect(row).toHaveAttribute("data-action-state", "idle");
    await expect(row.locator("[data-queue-result]")).toHaveText("已通过");
  });
});

test("operator cookie session skips Mini App exchange", async ({ page }) => {
  const requests: string[] = [];

  await page.context().addCookies([
    {
      name: "vestibule_console_session",
      value: "operator-session",
      url: "http://127.0.0.1:4173"
    }
  ]);
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
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

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await page.goto("/");
  await expect(page).toHaveURL(/\/groups$/);
  await expect(page.locator("[data-groups-page]")).toBeVisible();
  expect(requests).toEqual(["GET /api/session", "GET /api/chats"]);
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
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });

  await page.goto("/groups");
  await expect(page.locator("[data-groups-page]")).toHaveAttribute(
    "data-groups-source",
    "api"
  );
  expect(await page.evaluate(() => document.activeElement?.tagName)).toBe("BODY");

  let focusedGroupId: string | null = null;
  for (let index = 0; index < 12; index += 1) {
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
  await page.getByRole("combobox", { name: "语言" }).selectOption("en");
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

test("a failed release restores pending state and remaining time", async ({
  page
}) => {
  await openFixtureQueue(page);

  const row = rowFor(page, "@retry_release");
  const result = row.locator("[data-queue-result]");
  await expect(row).toHaveAttribute("data-result", "pending");
  await expect(result).toContainText("等待中 3:39");

  await row.getByRole("button", { name: "放行 @retry_release" }).click();
  await expect(row).toHaveAttribute("data-result", "approved");

  const feedback = page.locator(
    '[data-queue-feedback][data-feedback-kind="releaseFailure"]'
  );
  await expect(feedback).toContainText("没能放行 @retry_release");
  await expect(row).toHaveAttribute("data-action-state", "idle");
  await expect(row).toHaveAttribute("data-result", "pending");
  await expect(result).toContainText("等待中 3:39");
});

test("revoke does nothing without confirmation", async ({ page }) => {
  await openFixtureQueue(page);

  const row = rowFor(page, "@spam_ad_01");
  await expect(row).toHaveAttribute("data-result", "banned");

  await row
    .getByRole("button", { name: "撤销 @spam_ad_01 的封禁" })
    .click();

  const dialog = page.getByRole("dialog", {
    name: "撤销 @spam_ad_01 的封禁"
  });
  await expect(dialog).toBeVisible();
  await expect(row).toHaveAttribute("data-action-state", "idle");
  await expect(row).toHaveAttribute("data-result", "banned");
  await expect(page.locator("[data-queue-feedback]")).toHaveCount(0);

  await dialog.getByRole("button", { name: "取消" }).click();
  await expect(dialog).not.toBeVisible();
  await page.waitForTimeout(800);
  await expect(row).toHaveAttribute("data-action-state", "idle");
  await expect(row).toHaveAttribute("data-result", "banned");
  await expect(page.locator("[data-queue-feedback]")).toHaveCount(0);
});
