import { expect, test, type Page, type Route } from "@playwright/test";

const actorID = "9000000901";
const groupID = "-1009000000902";

const blockedRetryScreens = [
  ["queue", "/queue"],
  ["audit", "/audit"],
  ["moderation", "/moderation"],
  ["bypass", "/bypass"],
  ["verification", "/verification"],
  ["questions", "/questions"],
  ["capabilities", "/capabilities"],
  ["home", "/home"],
  ["statistics", "/stats"],
  ["messages", "/messages"],
  ["feeds", "/feeds"],
  ["diagnostics", "/diagnostics"]
] as const;

const groupRetryScreens = blockedRetryScreens.filter(
  ([name]) => name !== "feeds" && name !== "diagnostics"
);

const processRetryScreens = [
  ["feeds", "/feeds", "/api/process/settings", "process_settings_unavailable"],
  ["diagnostics", "/diagnostics", "/api/status", "diagnostics_unavailable"]
] as const;

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function expectRetryRequest(page: Page, path: string, requests: () => number): Promise<void> {
  await page.goto(path);
  const retry = page.getByRole("button", { name: "重试", exact: true });
  await expect(retry).toBeVisible();
  await retry.click();
  await expect.poll(requests).toBe(2);
}

for (const [name, path] of blockedRetryScreens) {
  test(`${name} retry restarts a blocked console session`, async ({ page }) => {
    let sessionRequests = 0;
    await page.route("**/api/**", async (route) => {
      const request = route.request();
      const requestPath = new URL(request.url()).pathname;
      if (requestPath === "/api/session" && request.method() === "GET") {
        sessionRequests += 1;
        await fulfillJSON(route, { error: { code: "authentication_unavailable" } }, 503);
        return;
      }
      throw new Error(`Unexpected API request: ${request.method()} ${requestPath}`);
    });

    await expectRetryRequest(page, path, () => sessionRequests);
  });
}

for (const [name, path] of groupRetryScreens) {
  test(`${name} retry reloads an unavailable group list`, async ({ page }) => {
    let groupRequests = 0;
    await page.route("**/api/**", async (route) => {
      const request = route.request();
      const requestPath = new URL(request.url()).pathname;
      if (requestPath === "/api/session" && request.method() === "GET") {
        await fulfillJSON(route, {
          subject: { telegram_id: actorID, role: "manager" },
          expires_at: "2026-09-02T12:00:00Z",
          csrf_token: "session-retry-csrf"
        });
        return;
      }
      if (requestPath === "/api/chats" && request.method() === "GET") {
        groupRequests += 1;
        await fulfillJSON(route, { error: { code: "chat_access_unavailable" } }, 503);
        return;
      }
      throw new Error(`Unexpected API request: ${request.method()} ${requestPath}`);
    });

    await expectRetryRequest(page, path, () => groupRequests);
  });
}

for (const [name, path, endpoint, errorCode] of processRetryScreens) {
  test(`${name} retries its own failed read when the group list is unavailable`, async ({ page }) => {
    let groupRequests = 0;
    let processRequests = 0;
    await page.route("**/api/**", async (route) => {
      const request = route.request();
      const requestPath = new URL(request.url()).pathname;
      if (requestPath === "/api/session" && request.method() === "GET") {
        await fulfillJSON(route, {
          subject: { telegram_id: actorID, role: "operator" },
          expires_at: "2026-09-02T12:00:00Z",
          csrf_token: "process-retry-csrf"
        });
        return;
      }
      if (requestPath === "/api/chats" && request.method() === "GET") {
        groupRequests += 1;
        await fulfillJSON(route, { error: { code: "chat_access_unavailable" } }, 503);
        return;
      }
      if (requestPath === endpoint && request.method() === "GET") {
        processRequests += 1;
        await fulfillJSON(route, { error: { code: errorCode } }, 503);
        return;
      }
      throw new Error(`Unexpected API request: ${request.method()} ${requestPath}`);
    });

    await page.goto(path);
    const retry = page.getByRole("button", { name: "重试", exact: true });
    await expect(retry).toBeVisible();
    await retry.click();
    await expect.poll(() => processRequests).toBe(2);
    expect(groupRequests).toBe(1);
  });
}
