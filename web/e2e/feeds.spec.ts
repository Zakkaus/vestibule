import { expect, test, type Page, type Route } from "@playwright/test";

const actorID = "9000000201";
const selectedGroupID = "-1009000000202";

type ConsoleRole = "manager" | "operator";
type ProcessSettingsHandler = (route: Route) => Promise<void>;

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockFeedsTransport(
  page: Page,
  role: ConsoleRole,
  processSettings: ProcessSettingsHandler
): Promise<string[]> {
  const processMethods: string[] = [];
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role },
        expires_at: "2026-09-02T12:00:00Z",
        csrf_token: "feeds-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID }] });
      return;
    }
    if (path === "/api/process/settings") {
      processMethods.push(request.method());
      await processSettings(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
  return processMethods;
}

async function openLoadedFeeds(page: Page, body: unknown): Promise<string[]> {
  const processMethods = await mockFeedsTransport(page, "operator", async (route) => {
    if (route.request().method() !== "GET") {
      throw new Error(`Unexpected process settings write: ${route.request().method()}`);
    }
    await fulfillJSON(route, body);
  });
  await page.goto(`/feeds?group=${selectedGroupID}`);
  await expect(page.locator("[data-feeds-page]")).toHaveAttribute("data-feeds-state", "loaded");
  return processMethods;
}

const configuredProcessSettings = {
  feeds: {
    value: [
      {
        chat_id: -1009000000203,
        lang: "en",
        interval_seconds: 600,
        bugs: false,
        news: true,
        bug_product: "Gentoo Linux",
        bug_component: "Portage",
        silent_bugs: true
      },
      {
        chat_id: -1009000000204,
        lang: "",
        interval_seconds: 1800,
        bugs: null,
        news: null,
        bug_product: "",
        bug_component: "",
        silent_bugs: null
      }
    ],
    source: "user file"
  },
  news_url: {
    value: "https://example.invalid/news-items.xml",
    source: "factory default"
  },
  overlays: {
    value: [
      { name: "gentoo", repo: "gentoo/gentoo", branch: "master" },
      { name: "guru", repo: "gentoo/guru", branch: "" }
    ],
    source: "user file"
  },
  stats_timezone: { value: "Asia/Shanghai", source: "user file" }
} as const;

test("feed delivery renders process values, array records, and API provenance without controls", async ({
  page
}) => {
  const processMethods = await openLoadedFeeds(page, configuredProcessSettings);
  const screen = page.locator("[data-feeds-page]");

  await expect(screen.locator("[data-feeds-readonly]")).toContainText("此屏没有写入路径");
  await expect(screen.locator("input, textarea, select, button")).toHaveCount(0);
  await expect(screen.getByText("保存", { exact: true })).toHaveCount(0);
  expect(processMethods).toEqual(["GET"]);

  const feedSection = screen.locator('[data-feeds-section="feeds"]');
  await expect(feedSection).toHaveAttribute("data-process-setting-source", "user file");
  await expect(feedSection).toContainText("来源：由文件管理");
  const feeds = feedSection.locator("[data-feed-item]");
  await expect(feeds).toHaveCount(2);
  await expect(feeds.nth(0)).toContainText("-1009000000203");
  await expect(feeds.nth(0)).toContainText("600 秒");
  await expect(feeds.nth(0)).toContainText("Gentoo Linux");
  await expect(feeds.nth(0)).toContainText("Portage");
  await expect(feeds.nth(0)).toContainText("关闭");
  await expect(feeds.nth(0)).toContainText("开启");
  await expect(feeds.nth(1)).toContainText("简体中文（默认）");
  await expect(feeds.nth(1)).toContainText("开启（默认）");
  await expect(feeds.nth(1)).toContainText("关闭（默认）");
  await expect(feeds.nth(1)).toContainText("全部产品");
  await expect(feeds.nth(1)).toContainText("全部组件");

  const newsURL = screen.locator('[data-feeds-section="news-url"]');
  await expect(newsURL).toHaveAttribute("data-process-setting-source", "factory default");
  await expect(newsURL).toContainText("来源：出厂默认");
  await expect(newsURL.locator("code")).toHaveText("https://example.invalid/news-items.xml");

  const overlays = screen.locator('[data-feeds-section="overlays"]');
  await expect(overlays).toHaveAttribute("data-process-setting-source", "user file");
  await expect(overlays.locator("[data-overlay-item]")).toHaveCount(2);
  await expect(overlays.locator("[data-overlay-item]").nth(0)).toContainText("gentoo/gentoo");
  await expect(overlays.locator("[data-overlay-item]").nth(0)).toContainText("master");
  await expect(overlays.locator("[data-overlay-item]").nth(1)).toContainText("gentoo/guru");
  await expect(overlays.locator("[data-overlay-item]").nth(1)).toContainText("master（默认）");
});

test("feed delivery identifies operator-only access instead of reporting a load failure", async ({
  page
}) => {
  const processMethods = await mockFeedsTransport(page, "manager", async (route) => {
    await fulfillJSON(route, { error: { code: "process_access_denied" } }, 403);
  });

  await page.goto("/feeds");
  const screen = page.locator("[data-feeds-page]");
  await expect(screen).toHaveAttribute("data-feeds-state", "access-denied");
  await expect(screen.getByRole("heading", { name: "没有权限查看进程设置" })).toBeVisible();
  await expect(screen).toContainText("此屏仅向运维开放");
  await expect(screen.getByText("无法读取订阅推送配置", { exact: true })).toHaveCount(0);
  await expect(screen.getByRole("button", { name: "重试" })).toHaveCount(0);
  expect(processMethods).toEqual(["GET"]);
});

test("feed delivery keeps empty process arrays visible with their sources", async ({ page }) => {
  const processMethods = await openLoadedFeeds(page, {
    feeds: { value: [], source: "factory default" },
    news_url: { value: "", source: "factory default" },
    overlays: { value: [], source: "factory default" },
    stats_timezone: { value: "", source: "factory default" }
  });
  const screen = page.locator("[data-feeds-page]");

  await expect(screen.locator("[data-feeds-empty]")).toContainText("未配置订阅目标");
  await expect(screen.locator("[data-overlays-empty]")).toContainText("未配置仓库");
  await expect(screen.locator("[data-news-url-value]")).toContainText("未配置新闻源地址");
  await expect(screen.locator("[data-feed-item]")).toHaveCount(0);
  await expect(screen.locator("[data-overlay-item]")).toHaveCount(0);
  await expect(screen.locator('[data-process-setting-source="factory default"]')).toHaveCount(3);
  expect(processMethods).toEqual(["GET"]);
});
