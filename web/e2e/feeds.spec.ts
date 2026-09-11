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
        is_owner: false,
        csrf_token: "feeds-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [
        { id: selectedGroupID, title: "Gentoo-zh Community", owner: null, administrators: [], administrators_status: "unavailable" },
        { id: "-1009000000203", title: "Gentoo Package Updates", owner: null, administrators: [], administrators_status: "unavailable" },
        { id: "-1009000000204", title: "Linux News", owner: null, administrators: [], administrators_status: "unavailable" }
      ] });
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
        silent_bugs: true,
        github_repos: [
          { repo: "gentoo-zh/overlay", branch: "main", issues: true, pulls: false },
          { repo: "gentoo-zh/overlay", branch: "release", issues: false, pulls: true },
          { repo: "gentoo-zh/overlay", issues: null, pulls: null }
        ]
      },
      {
        chat_id: -1009000000204,
        lang: "",
        interval_seconds: 1800,
        bugs: null,
        news: null,
        bug_product: "",
        bug_component: "",
        silent_bugs: null,
        github_repos: [{ repo: "Zakkaus/vestibule", branch: "v5/next", issues: true, pulls: true }]
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

  await expect(screen.locator("[data-feeds-readonly]")).toContainText("订阅设置无法在控制台修改");
  await expect(screen.locator("input, textarea, select, button")).toHaveCount(0);
  await expect(screen.getByText("保存", { exact: true })).toHaveCount(0);
  await expect(screen).not.toContainText("github_atom_base");
  expect(processMethods).toEqual(["GET"]);

  const feedSection = screen.locator('[data-feeds-section="feeds"]');
  await expect(feedSection).toHaveAttribute("data-process-setting-source", "user file");
  await expect(feedSection).toContainText("来源：配置文件");
  const feeds = feedSection.locator("[data-feed-item]");
  await expect(feeds).toHaveCount(2);
  await expect(feeds.nth(0)).toContainText("Gentoo Package Updates");
  await expect(feeds.nth(1)).toContainText("Linux News");
  await expect(screen).not.toContainText(/-100\d+/);
  const githubRepos = feeds.nth(0).locator("[data-feed-value]").filter({ hasText: "GitHub 仓库" });
  await expect(githubRepos).toHaveCount(3);
  await expect(githubRepos.nth(0)).toContainText("gentoo-zh/overlay");
  await expect(githubRepos.nth(0)).toContainText("main");
  await expect(githubRepos.nth(1)).toContainText("release");
  await expect(githubRepos.nth(2)).toContainText("跟随默认分支");
  const secondGitHubRepos = feeds.nth(1).locator("[data-feed-value]").filter({ hasText: "GitHub 仓库" });
  await expect(secondGitHubRepos).toHaveCount(1);
  await expect(secondGitHubRepos).toContainText("Zakkaus/vestibule");
  await expect(secondGitHubRepos).toContainText("v5/next");
  await expect(feeds.nth(0).getByText("Issue 推送", { exact: true })).toHaveCount(3);
  await expect(feeds.nth(0).getByText("PR 推送", { exact: true })).toHaveCount(3);
  await expect(feeds.nth(0).getByText("Issue 推送", { exact: true }).nth(0).locator("..")).toContainText("开启");
  await expect(feeds.nth(0).getByText("PR 推送", { exact: true }).nth(0).locator("..")).toContainText("关闭");
  await expect(feeds.nth(0).getByText("Issue 推送", { exact: true }).nth(2).locator("..")).toContainText("关闭（默认）");
  await expect(feeds.nth(1).getByText("Issue 推送", { exact: true })).toHaveCount(1);
  await expect(feeds.nth(1).getByText("PR 推送", { exact: true })).toHaveCount(1);
  await expect(feeds.nth(0)).toContainText("600 秒");
  await expect(feeds.nth(0)).toContainText("Gentoo Linux");
  await expect(feeds.nth(0)).toContainText("Portage");
  await expect(feeds.nth(0)).toContainText("已关闭");
  await expect(feeds.nth(0)).toContainText("已开启");
  await expect(feeds.nth(1)).toContainText("简体中文（默认）");
  await expect(feeds.nth(1)).toContainText("已开启（默认）");
  await expect(feeds.nth(1)).toContainText("已关闭（默认）");
  await expect(feeds.nth(1)).toContainText("全部产品");
  await expect(feeds.nth(1)).toContainText("全部组件");

  const newsURL = screen.locator('[data-feeds-section="news-url"]');
  await expect(newsURL).toHaveAttribute("data-process-setting-source", "factory default");
  await expect(newsURL).toContainText("来源：程序默认值");
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
  await expect(screen.getByRole("heading", { name: "无权查看进程设置" })).toBeVisible();
  await expect(screen).toContainText("此页面仅供运维人员使用");
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
  await expect(screen.locator("[data-overlays-empty]")).toContainText("未配置 overlay 仓库");
  await expect(screen.locator("[data-news-url-value]")).toContainText("未配置新闻源地址");
  await expect(screen.locator("[data-feed-item]")).toHaveCount(0);
  await expect(screen.locator("[data-overlay-item]")).toHaveCount(0);
  await expect(screen.locator('[data-process-setting-source="factory default"]')).toHaveCount(3);
  expect(processMethods).toEqual(["GET"]);
});

test("feed delivery accepts missing, null, and empty GitHub repository lists", async ({ page }) => {
  const processMethods = await openLoadedFeeds(page, {
    feeds: {
      value: [
        {
          chat_id: -1009000000301,
          lang: "en",
          interval_seconds: 600,
          bugs: false,
          news: false,
          bug_product: "",
          bug_component: "",
          silent_bugs: false
        },
        {
          chat_id: -1009000000302,
          lang: "en",
          interval_seconds: 600,
          bugs: false,
          news: false,
          bug_product: "",
          bug_component: "",
          silent_bugs: false,
          github_repos: null
        },
        {
          chat_id: -1009000000303,
          lang: "en",
          interval_seconds: 600,
          bugs: false,
          news: false,
          bug_product: "",
          bug_component: "",
          silent_bugs: false,
          github_repos: []
        }
      ],
      source: "user file"
    },
    news_url: { value: "", source: "factory default" },
    overlays: { value: [], source: "factory default" },
    stats_timezone: { value: "", source: "factory default" }
  });
  const screen = page.locator("[data-feeds-page]");
  const feeds = screen.locator("[data-feed-item]");

  await expect(feeds).toHaveCount(3);
  for (const feed of await feeds.all()) {
    await expect(feed.getByText("GitHub 仓库", { exact: true })).toHaveCount(0);
  }
  expect(processMethods).toEqual(["GET"]);
});
