import { expect, test, type Page, type Route } from "@playwright/test";

const actorID = "9000000201";
const selectedGroupID = "-1009000000202";

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({ status, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
}

async function mockFeedsTransport(page: Page, role: "manager" | "operator", feeds: unknown, processStatus = 200): Promise<{ feed: number; process: number }> {
  const requests = { feed: 0, process: 0 };
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);
    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, { subject: { telegram_id: actorID, role }, expires_at: "2026-09-02T12:00:00Z", is_owner: false, csrf_token: "feeds-csrf" });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID, title: "Gentoo-zh Community", owner: null, administrators: [], administrators_status: "unavailable" }] });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/feeds` && request.method() === "GET") {
      requests.feed += 1;
      await fulfillJSON(route, feeds);
      return;
    }
    if (path === "/api/process/settings" && request.method() === "GET") {
      requests.process += 1;
      await fulfillJSON(route, processStatus === 200 ? {
        news_url: { value: "https://www.gentoo.org/feeds/news.xml", source: "factory default" },
        overlays: { value: [{ name: "gentoo", repo: "https://github.com/gentoo/gentoo.git", branch: "master" }], source: "user file" }
      } : { error: { code: "process_access_denied" } }, processStatus);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
  return requests;
}

const feedPayload = {
  revision: 4,
  feed: {
    lang: { value: "en", source: "chat override" },
    interval_seconds: { value: 600, source: "chat override" },
    bugs: { value: false, source: "factory default" },
    news: { value: true, source: "chat override" },
    bug_product: { value: "Gentoo Linux", source: "chat override" },
    bug_component: { value: "Portage", source: "chat override" },
    silent_bugs: { value: true, source: "chat override" }
  },
  github_repos: { value: [{ repo: "gentoo-zh/overlay", branch: "main", issues: true, pulls: false }], source: "chat override" }
} as const;

test("feeds screen reads the selected group endpoint and stays read-only", async ({ page }) => {
  const requests = await mockFeedsTransport(page, "operator", feedPayload);
  await page.goto(`/feeds?group=${selectedGroupID}`);
  const screen = page.locator("[data-feeds-page]");
  await expect(screen).toHaveAttribute("data-feeds-state", "loaded");
  await expect(screen.locator("[data-feeds-readonly]")).toBeVisible();
  await expect(screen.locator("input, textarea, select, button")).toHaveCount(0);
  await expect(screen).toContainText("600 秒");
  await expect(screen).toContainText("Gentoo Linux");
  await expect(screen).toContainText("gentoo-zh/overlay");
  await expect(screen).toContainText("https://www.gentoo.org/feeds/news.xml");
  await expect(screen).toContainText("https://github.com/gentoo/gentoo.git");
  await expect(screen).toContainText("来源：群覆盖");
  expect(requests.feed).toBe(1);
  expect(requests.process).toBe(1);
});

test("feeds screen preserves the group read when operator process settings are forbidden", async ({ page }) => {
  const requests = await mockFeedsTransport(page, "manager", feedPayload, 403);
  await page.goto(`/feeds?group=${selectedGroupID}`);
  await expect(page.locator("[data-feeds-page]")).toHaveAttribute("data-feeds-state", "loaded");
  await expect(page.locator("[data-feed-item]")).toContainText("gentoo-zh/overlay");
  await expect(page.locator('[data-feeds-section="news-url"]')).toHaveCount(0);
  expect(requests.feed).toBe(1);
  expect(requests.process).toBe(1);
});
