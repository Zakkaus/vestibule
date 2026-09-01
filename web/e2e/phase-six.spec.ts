import { expect, test, type Page } from "@playwright/test";

const selectedGroupId = "-1001163306055";

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
    await expect(page.locator("[data-groups-page]")).toBeVisible();
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
    await expect(page.locator("[data-group-row][data-selected]")).toContainText(
      "Gentoo 中文社区"
    );
  });

  await test.step("open the waiting queue", async () => {
    await page.getByRole("link", { name: "等待队列" }).click();
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
