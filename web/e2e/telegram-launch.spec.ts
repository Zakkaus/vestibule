import { expect, test, type Page, type Route } from "@playwright/test";

const groupID = "-1009000000902";
const groupTitle = "Selected launch group";
const initData = "query_id=telegram-test&user=%7B%22id%22%3A9000000901%7D&hash=signature%2Bvalue";

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockLaunchSession(page: Page, sessionBodies: unknown[], sessionError?: string): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, { error: { code: "authentication_expired" } }, 401);
    } else if (path === "/api/session" && request.method() === "POST") {
      sessionBodies.push(request.postDataJSON());
      if (sessionError) {
        await fulfillJSON(route, { error: { code: sessionError } }, 409);
        return;
      }
      await fulfillJSON(route, {
        subject: { telegram_id: "9000000901", role: "manager" },
        expires_at: "2030-09-04T12:00:00Z",
        is_owner: false,
        csrf_token: "telegram-launch-csrf"
      }, 201);
    } else if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [
        { id: "-1009000000903", title: "Other group", owner: null, administrators: [], administrators_status: "unavailable" },
        { id: groupID, title: groupTitle, owner: null, administrators: [], administrators_status: "unavailable" }
      ] });
    } else if (path === "/api/instance" && request.method() === "GET") {
      await fulfillJSON(route, { bot_username: "example_bot" });
    } else {
      throw new Error(`Unexpected API request: ${request.method()} ${path}`);
    }
  });
}

for (const source of ["fragment", "query", "fragment over query", "WebApp global"] as const) {
  test(`Mini App signs in from ${source} and removes launch parameters`, async ({ page }) => {
    const sessionBodies: unknown[] = [];
    await mockLaunchSession(page, sessionBodies);
    const query = new URLSearchParams({ group: groupID, keep: "query value" });
    const fragment = new URLSearchParams({ keep: "fragment value" });
    const expectedInitData = source === "WebApp global" ? `${initData}&source=sdk` : initData;
    if (source === "query" || source === "fragment over query") {
      query.set("tgWebAppData", source === "query" ? initData : `${initData}&source=query`);
      query.set("tgWebAppPlatform", "tdesktop");
    }
    if (source !== "query") {
      fragment.set("tgWebAppData", initData);
      fragment.set("tgWebAppVersion", "9.0");
      fragment.append("tgWebAppVersion", "9.1");
      fragment.set("tgWebAppThemeParams", "{}");
    }
    if (source === "WebApp global") {
      await page.addInitScript((data) => {
        Object.defineProperty(window, "Telegram", {
          configurable: true,
          value: { WebApp: { initData: data } }
        });
      }, expectedInitData);
    }

    await page.goto(`/groups?${query.toString()}#${fragment.toString()}`);
    await expect(page.locator("[data-groups-page]")).toHaveAttribute("data-groups-state", "populated");
    await expect(page.locator(`[data-group-row][data-group-chat-id="${groupID}"]`)).toHaveAttribute("aria-current", "true");
    await expect(page.locator("[data-group-row][data-selected]").getByRole("heading", { level: 2 })).toHaveText(groupTitle);
    expect(sessionBodies).toEqual([{ init_data: expectedInitData }]);
    const cleaned = new URL(page.url());
    expect(cleaned.searchParams.get("group")).toBe(groupID);
    expect(cleaned.searchParams.get("keep")).toBe("query value");
    expect(new URLSearchParams(cleaned.hash.slice(1)).get("keep")).toBe("fragment value");
    expect(cleaned.search + cleaned.hash).not.toContain("tgWebApp");
  });
}

for (const path of [`/groups?group=${groupID}`, "/missing"]) {
  test(`a replayed Mini App launch at ${path} requires reopening from Telegram`, async ({ page }) => {
    const sessionBodies: unknown[] = [];
    await mockLaunchSession(page, sessionBodies, "init_data_replayed");
    await page.goto(`${path}#${new URLSearchParams({ tgWebAppData: initData }).toString()}`);
    const entry = page.locator("[data-entry-page]");
    await expect(entry).toHaveAttribute("data-entry-state", "init-data-replayed");
    await expect(entry.getByRole("alert")).toContainText("Telegram");
    await expect(entry.getByRole("button")).toHaveCount(0);
    await expect(page.locator("[data-app-shell]")).toHaveAttribute("data-shell-variant", "entry");
    expect(sessionBodies).toEqual([{ init_data: initData }]);
    expect(new URL(page.url()).hash).not.toContain("tgWebAppData");
  });
}
