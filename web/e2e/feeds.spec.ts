import { expect, test, type Locator, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

const actorID = "9000000201";
const selectedGroupID = "-1009000000202";

type FeedSource = "factory default" | "user file" | "chat override";
type FeedLanguage = "" | "zh" | "zh-Hant" | "en" | "ja" | "ru";

type SourcedSetting<T> = Readonly<{
  value: T;
  source: FeedSource;
}>;

type FeedValues = Readonly<{
  lang: FeedLanguage;
  interval_seconds: number;
  bugs: boolean;
  news: boolean;
  bug_product: string;
  bug_component: string;
  silent_bugs: boolean;
}>;

type GitHubRepo = Readonly<{
  repo: string;
  branch: string;
  issues: boolean;
  pulls: boolean;
}>;

type FeedResponse = Readonly<{
  revision: number;
  feed: Readonly<{
    lang: SourcedSetting<FeedLanguage>;
    interval_seconds: SourcedSetting<number>;
    bugs: SourcedSetting<boolean>;
    news: SourcedSetting<boolean>;
    bug_product: SourcedSetting<string>;
    bug_component: SourcedSetting<string>;
    silent_bugs: SourcedSetting<boolean>;
  }>;
  github_repos: SourcedSetting<readonly GitHubRepo[]>;
}>;

type FeedWriteBody = FeedValues & Readonly<{
  expected_revision: number;
  github_repos: readonly GitHubRepo[];
}>;

type FeedResponseOptions = Readonly<{
  revision?: number;
  source?: FeedSource;
  values?: Partial<FeedValues>;
  githubRepos?: readonly GitHubRepo[];
  githubReposSource?: FeedSource;
}>;

type FeedReadHandler = (route: Route, requestNumber: number) => Promise<void>;
type FeedWriteHandler = (route: Route, requestNumber: number) => Promise<void>;

type FeedTransportOptions = Readonly<{
  role?: "manager" | "operator";
  readFeed?: FeedReadHandler;
  writeFeed?: FeedWriteHandler;
  processStatus?: number;
}>;

type FeedRequests = {
  feed: number;
  process: number;
  write: number;
};

const defaultFeedValues: FeedValues = {
  lang: "zh",
  interval_seconds: 600,
  bugs: false,
  news: false,
  bug_product: "",
  bug_component: "",
  silent_bugs: false
};

const defaultGitHubRepos: readonly GitHubRepo[] = [
  { repo: "gentoo-zh/overlay", branch: "main", issues: true, pulls: false }
];

const processPayload = {
  news_url: { value: "https://www.gentoo.org/feeds/news.xml", source: "factory default" },
  overlays: {
    value: [{ name: "gentoo", repo: "https://github.com/gentoo/gentoo.git", branch: "master" }],
    source: "user file"
  }
} as const;

function sourced<T>(value: T, source: FeedSource): SourcedSetting<T> {
  return { value, source };
}

function feedResponse(options: FeedResponseOptions = {}): FeedResponse {
  const source = options.source ?? "chat override";
  const values = { ...defaultFeedValues, ...options.values };
  const githubRepos = options.githubRepos ?? defaultGitHubRepos;
  return {
    revision: options.revision ?? 4,
    feed: {
      lang: sourced(values.lang, source),
      interval_seconds: sourced(values.interval_seconds, source),
      bugs: sourced(values.bugs, source),
      news: sourced(values.news, source),
      bug_product: sourced(values.bug_product, source),
      bug_component: sourced(values.bug_component, source),
      silent_bugs: sourced(values.silent_bugs, source)
    },
    github_repos: sourced(githubRepos, options.githubReposSource ?? source)
  };
}

function feedWriteBody(
  expectedRevision: number,
  values: FeedValues,
  githubRepos: readonly GitHubRepo[]
): FeedWriteBody {
  return { expected_revision: expectedRevision, ...values, github_repos: githubRepos };
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockFeedsTransport(
  page: Page,
  options: FeedTransportOptions = {}
): Promise<FeedRequests> {
  const requests: FeedRequests = { feed: 0, process: 0, write: 0 };
  const readFeed =
    options.readFeed ??
    (async (route) => {
      await fulfillJSON(route, feedResponse());
    });
  const processStatus = options.processStatus ?? 200;

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: options.role ?? "operator" },
        expires_at: "2026-09-02T12:00:00Z",
        is_owner: false,
        csrf_token: "feeds-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, {
        chats: [
          {
            id: selectedGroupID,
            title: "Gentoo-zh Community",
            owner: null,
            administrators: [],
            administrators_status: "unavailable"
          }
        ]
      });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/feeds` && request.method() === "GET") {
      requests.feed += 1;
      await readFeed(route, requests.feed);
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/feeds` && request.method() === "PUT") {
      requests.write += 1;
      if (!options.writeFeed) {
        throw new Error("Unexpected feeds write in a read-only acceptance journey");
      }
      await options.writeFeed(route, requests.write);
      return;
    }
    if (path === "/api/process/settings" && request.method() === "GET") {
      requests.process += 1;
      await fulfillJSON(
        route,
        processStatus === 200
          ? processPayload
          : { error: { code: "process_access_denied" } },
        processStatus
      );
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
  return requests;
}

function feedSetting(page: Page, name: keyof FeedValues): Locator {
  return page.locator(`[data-feed-setting=${JSON.stringify(name)}]`);
}

function settingControl(page: Page, name: keyof FeedValues): Locator {
  return feedSetting(page, name)
    .locator("input, [data-slot='select-trigger'], [role='switch']")
    .first();
}

function repositoryRow(page: Page, index: number): Locator {
  return page.locator(`[data-feed-repository=${JSON.stringify(String(index))}]`);
}

function feedsSaveBar(page: Page): Locator {
  return page.locator("[data-feeds-savebar]");
}

function feedsSaveButton(page: Page): Locator {
  return feedsSaveBar(page).getByRole("button").last();
}

async function openFeeds(
  page: Page,
  options: FeedTransportOptions = {}
): Promise<FeedRequests> {
  const requests = await mockFeedsTransport(page, options);
  await page.goto(`/feeds?group=${selectedGroupID}`);
  await expect(page.locator("[data-feeds-page]")).toHaveAttribute(
    "data-feeds-state",
    "loaded"
  );
  await expect(page.locator("[data-feeds-form]")).toBeVisible();
  return requests;
}

async function expectLinkedFieldError(field: Locator, control: Locator): Promise<void> {
  const error = field.getByRole("alert");
  await expect(error).toHaveCount(1);
  await expect(error).toBeVisible();
  const errorID = await error.getAttribute("id");
  expect(errorID).toBeTruthy();
  const describedBy = await control.getAttribute("aria-describedby");
  expect(describedBy?.split(/\s+/)).toContain(errorID);
}

test("feeds saves edited settings through the shared CSRF transport", async ({ page }) => {
  let markWriteRequested!: () => void;
  let releaseWrite!: () => void;
  const writeRequested = new Promise<void>((resolve) => {
    markWriteRequested = resolve;
  });
  const writeResponse = new Promise<void>((resolve) => {
    releaseWrite = resolve;
  });
  const editedValues: FeedValues = {
    lang: "en",
    interval_seconds: 900,
    bugs: true,
    news: true,
    bug_product: "Gentoo Linux",
    bug_component: "Portage",
    silent_bugs: true
  };
  const editedRepos: readonly GitHubRepo[] = [
    { repo: "gentoo/gentoo", branch: "main", issues: true, pulls: true }
  ];

  const requests = await openFeeds(page, {
    role: "operator",
    readFeed: async (route) => fulfillJSON(route, feedResponse()),
    writeFeed: async (route) => {
      const request = route.request();
      expect(request.headers()["x-csrf-token"]).toBe("feeds-csrf");
      expect(request.postDataJSON()).toEqual(feedWriteBody(4, editedValues, editedRepos));
      markWriteRequested();
      await writeResponse;
      await fulfillJSON(
        route,
        feedResponse({
          revision: 5,
          values: editedValues,
          githubRepos: editedRepos
        })
      );
    }
  });
  await expect(page.getByRole("heading", { name: "新闻源地址" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "仓库与分支" })).toBeVisible();

  await selectAppOption(page.locator("#feeds-language"), "en");
  await page.locator("#feeds-interval-seconds").fill("900");
  await feedSetting(page, "bugs").getByRole("switch").click();
  await feedSetting(page, "news").getByRole("switch").click();
  await feedSetting(page, "silent_bugs").getByRole("switch").click();
  await feedSetting(page, "bug_product").getByRole("textbox").fill("Gentoo Linux");
  await feedSetting(page, "bug_component").getByRole("textbox").fill("Portage");

  await repositoryRow(page, 0).getByRole("button", { name: /移除/ }).click();
  await expect(page.locator("[data-feed-repository]")).toHaveCount(0);
  await page.getByRole("button", { name: /添加.*仓库/ }).click();
  const repository = repositoryRow(page, 0);
  await repository.locator("input").nth(0).fill("gentoo/gentoo");
  await repository.locator("input").nth(1).fill("main");
  await repository.getByRole("switch").nth(0).click();
  await repository.getByRole("switch").nth(1).click();

  await feedsSaveButton(page).click();
  await writeRequested;
  await expect(feedsSaveBar(page)).toHaveAttribute("data-save-state", "submitting");
  await expect(feedsSaveButton(page)).toHaveAttribute("aria-disabled", "true");
  releaseWrite();
  await expect(page.locator("[data-feeds-feedback]")).toHaveAttribute("role", "status");
  await expect(page.locator("[data-feeds-feedback]")).toContainText("订阅设置已保存");
  await expect(feedsSaveBar(page)).toContainText("没有未保存的修改");
  expect(requests.write).toBe(1);
});


test("feeds reloads the newer revision after a settings conflict", async ({ page }) => {
  let reads = 0;
  let markLatestRead!: () => void;
  const latestRead = new Promise<void>((resolve) => {
    markLatestRead = resolve;
  });
  const latestValues: FeedValues = {
    lang: "ja",
    interval_seconds: 120,
    bugs: true,
    news: false,
    bug_product: "Gentoo",
    bug_component: "Portage",
    silent_bugs: false
  };

  await openFeeds(page, {
    readFeed: async (route, requestNumber) => {
      reads = requestNumber;
      await fulfillJSON(
        route,
        requestNumber === 1
          ? feedResponse({ revision: 12 })
          : feedResponse({ revision: 13, values: latestValues, githubRepos: [] })
      );
      if (requestNumber === 2) markLatestRead();
    },
    writeFeed: async (route) => {
      expect(route.request().postDataJSON()).toMatchObject({ expected_revision: 12 });
      await fulfillJSON(route, { error: { code: "settings_conflict" } }, 409);
    }
  });

  await selectAppOption(page.locator("#feeds-language"), "en");
  await feedsSaveButton(page).click();
  await latestRead;
  await expect(page.locator("#feeds-language")).toHaveAttribute("data-value", "ja");
  await expect(page.locator("#feeds-interval-seconds")).toHaveValue("120");
  await expect(page.locator("[data-feeds-feedback]")).toBeVisible();
  await expect(page.locator("[data-feeds-feedback]")).toContainText("管理员");
  expect(reads).toBe(2);
});

test("feeds restores factory settings with an empty repository list", async ({ page }) => {
  const overriddenValues: FeedValues = {
    lang: "en",
    interval_seconds: 1800,
    bugs: true,
    news: true,
    bug_product: "Gentoo",
    bug_component: "Portage",
    silent_bugs: true
  };
  const factoryValues: FeedValues = {
    lang: "",
    interval_seconds: 300,
    bugs: false,
    news: false,
    bug_product: "",
    bug_component: "",
    silent_bugs: false
  };

  await openFeeds(page, {
    readFeed: async (route) =>
      fulfillJSON(
        route,
        feedResponse({
          revision: 22,
          values: overriddenValues,
          githubRepos: [
            { repo: "gentoo/gentoo", branch: "main", issues: true, pulls: true }
          ]
        })
      ),
    writeFeed: async (route) => {
      expect(route.request().postDataJSON()).toEqual(
        feedWriteBody(22, factoryValues, [])
      );
      await fulfillJSON(
        route,
        feedResponse({
          revision: 23,
          source: "factory default",
          values: factoryValues,
          githubRepos: []
        })
      );
    }
  });

  await page
    .locator("[data-feeds-form]")
    .getByRole("button", { name: /恢复.*(?:出厂|默认)/ })
    .click();
  await expect(page.locator("#feeds-language")).toHaveAttribute("data-value", "");
  await expect(page.locator("#feeds-interval-seconds")).toHaveValue("300");
  await expect(feedSetting(page, "bugs").getByRole("switch")).toHaveAttribute(
    "aria-checked",
    "false"
  );
  await expect(feedSetting(page, "news").getByRole("switch")).toHaveAttribute(
    "aria-checked",
    "false"
  );
  await expect(feedSetting(page, "silent_bugs").getByRole("switch")).toHaveAttribute(
    "aria-checked",
    "false"
  );
  await expect(feedSetting(page, "bug_product").getByRole("textbox")).toHaveValue("");
  await expect(feedSetting(page, "bug_component").getByRole("textbox")).toHaveValue("");
  await expect(page.locator("[data-feed-repository]")).toHaveCount(0);
  await feedsSaveButton(page).click();
  await expect(page.locator("[data-feeds-feedback]")).toBeVisible();
});

test("feeds maps every backend validation code to its field or save bar", async ({ page }) => {
  const validation = {
    error: { code: "invalid_request" },
    fields: [
      { name: "bug_product", code: "required_field" },
      { name: "interval_seconds", code: "invalid_interval" },
      { name: "lang", code: "invalid_language" },
      { name: "github_repos[0].repo", code: "invalid_repository" },
      { name: "github_repos[1]", code: "duplicate_repository" },
      { name: "expected_revision", code: "invalid_revision" }
    ]
  } as const;

  await openFeeds(page, {
    readFeed: async (route) =>
      fulfillJSON(
        route,
        feedResponse({
          githubRepos: [
            { repo: "gentoo/gentoo", branch: "main", issues: true, pulls: false },
            { repo: "gentoo-zh/overlay", branch: "main", issues: false, pulls: true }
          ]
        })
      ),
    writeFeed: async (route) => {
      expect(route.request().postDataJSON()).toMatchObject({ expected_revision: 4 });
      await fulfillJSON(route, validation, 400);
    }
  });

  await page.locator("#feeds-interval-seconds").fill("601");
  await feedsSaveButton(page).click();
  await expectLinkedFieldError(
    feedSetting(page, "bug_product"),
    settingControl(page, "bug_product")
  );
  await expect(feedSetting(page, "bug_product").getByRole("alert")).toContainText("此字段为必填项");
  await expectLinkedFieldError(
    feedSetting(page, "interval_seconds"),
    settingControl(page, "interval_seconds")
  );
  await expect(feedSetting(page, "interval_seconds").getByRole("alert")).toContainText("60 至 86400");
  await expectLinkedFieldError(feedSetting(page, "lang"), settingControl(page, "lang"));
  await expect(feedSetting(page, "lang").getByRole("alert")).toContainText("受支持的推送语言");
  await expectLinkedFieldError(
    repositoryRow(page, 0),
    repositoryRow(page, 0).locator("input").first()
  );
  await expect(repositoryRow(page, 0).getByRole("alert")).toContainText("owner/name");
  await expectLinkedFieldError(
    repositoryRow(page, 1),
    repositoryRow(page, 1).locator("input").first()
  );
  await expect(repositoryRow(page, 1).getByRole("alert")).toContainText("组合必须唯一");
  await expect(feedsSaveBar(page).getByRole("alert")).toContainText("设置已过期");
  await expect(page.locator("[data-feeds-feedback]")).toBeVisible();
});

test("feeds links repository toggle errors to their controls", async ({ page }) => {
  await openFeeds(page, {
    readFeed: async (route) =>
      fulfillJSON(
        route,
        feedResponse({
          githubRepos: [
            { repo: "gentoo/gentoo", branch: "main", issues: true, pulls: false },
            { repo: "gentoo-zh/overlay", branch: "main", issues: false, pulls: true }
          ]
        })
      ),
    writeFeed: async (route) =>
      fulfillJSON(
        route,
        {
          error: { code: "invalid_request" },
          fields: [
            { name: "github_repos[0].issues", code: "required_field" },
            { name: "github_repos[1].pulls", code: "required_field" }
          ]
        },
        400
      )
  });

  await page.locator("#feeds-interval-seconds").fill("601");
  await feedsSaveButton(page).click();
  await expectLinkedFieldError(
    repositoryRow(page, 0),
    repositoryRow(page, 0).getByRole("switch").nth(0)
  );
  await expectLinkedFieldError(
    repositoryRow(page, 1),
    repositoryRow(page, 1).getByRole("switch").nth(1)
  );
});

test("feeds rejects intervals outside the backend range before writing", async ({ page }) => {
  let writes = 0;
  const values: FeedValues = { ...defaultFeedValues };

  await openFeeds(page, {
    writeFeed: async (route) => {
      const body = route.request().postDataJSON() as FeedWriteBody;
      expect(body.interval_seconds).toBe([60, 86400][writes]);
      expect(body.expected_revision).toBe(4 + writes);
      writes += 1;
      await fulfillJSON(
        route,
        feedResponse({
          revision: 4 + writes,
          values: { ...values, interval_seconds: body.interval_seconds }
        })
      );
    }
  });

  const interval = page.locator("#feeds-interval-seconds");
  for (const invalid of [59, 86401]) {
    await interval.fill(String(invalid));
    await feedsSaveButton(page).click();
    await expect(interval).toHaveAttribute("aria-invalid", "true");
    await expect(feedSetting(page, "interval_seconds").getByRole("alert")).toBeVisible();
    expect(writes).toBe(0);
  }

  for (const [index, valid] of [60, 86400].entries()) {
    await interval.fill(String(valid));
    await feedsSaveButton(page).click();
    await expect.poll(() => writes).toBe(index + 1);
  }
});

test("feeds keeps the editable group form when process settings are forbidden", async ({ page }) => {
  const requests = await openFeeds(page, { role: "manager", processStatus: 403 });
  await expect(page.locator("[data-feeds-form]")).toBeVisible();
  await expect(page.locator('[data-feeds-section="news-url"]')).toHaveCount(0);
  await expect(page.locator('[data-feeds-section="overlays"]')).toHaveCount(0);
  expect(requests.feed).toBe(1);
  expect(requests.process).toBe(1);
});

test("feeds stacks field copy above controls on narrow screens", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openFeeds(page, { role: "manager", processStatus: 403 });

  const geometry = await feedSetting(page, "lang").evaluate((setting) => {
    const copy = setting.querySelector("[data-setting-copy]")?.getBoundingClientRect();
    const control = setting.querySelector("[data-setting-control]")?.getBoundingClientRect();
    return {
      stacked: Boolean(copy && control && control.top >= copy.bottom),
      readableCopy: Boolean(copy && copy.width >= 200),
      withinViewport: document.documentElement.scrollWidth <= window.innerWidth
    };
  });
  expect(geometry).toEqual({
    stacked: true,
    readableCopy: true,
    withinViewport: true
  });
});
