import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1001163306055";
const actorID = "741928306";

type Outcome = Readonly<{
  challenges: number;
  approved: number;
  declined: number;
  banned: number;
  expired: number;
  pass_rate: number;
}>;

type StatsResponse = Readonly<{
  range: Readonly<{ from: string; to: string; timezone: string }>;
  summary: Outcome;
  trend: readonly (Outcome & Readonly<{ date: string }>)[];
  interceptions: readonly Readonly<{ kind: string; count: number }>[];
}>;

type StatsHandler = (route: Route, query: URLSearchParams) => Promise<void>;

function outcome(overrides: Partial<Outcome> = {}): Outcome {
  return {
    challenges: 10,
    approved: 5,
    declined: 2,
    banned: 1,
    expired: 2,
    pass_rate: 0.5,
    ...overrides
  };
}

function statsResponse(
  query: URLSearchParams,
  overrides: Partial<StatsResponse> = {}
): StatsResponse {
  return {
    range: {
      from: query.get("from") ?? "2026-09-01",
      to: query.get("to") ?? "2026-09-08",
      timezone: query.get("timezone") ?? "UTC"
    },
    summary: outcome(),
    trend: [{ date: query.get("from") ?? "2026-09-01", ...outcome() }],
    interceptions: [{ kind: "kernel", count: 3 }],
    ...overrides
  };
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockStatsTransport(page: Page, handleStats: StatsHandler): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());

    if (url.pathname === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2026-09-01T02:00:00Z",
        csrf_token: "stats-csrf"
      });
      return;
    }
    if (url.pathname === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID }] });
      return;
    }
    if (url.pathname === `/api/chats/${selectedGroupID}/stats` && request.method() === "GET") {
      await handleStats(route, url.searchParams);
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${url.pathname}`);
  });
}

async function openLiveStats(page: Page, handleStats: StatsHandler): Promise<void> {
  await mockStatsTransport(page, handleStats);
  await page.goto(`/stats?group=${selectedGroupID}`);
  await expect(page.locator("[data-stats-page]")).toHaveAttribute("data-stats-state", "loaded");
}

test("statistics sends all required query parameters and renders an unknown verification kind", async ({ page }) => {
  const requests: URLSearchParams[] = [];

  await openLiveStats(page, async (route, query) => {
    requests.push(new URLSearchParams(query));
    for (const name of ["from", "to", "timezone"]) {
      expect(query.getAll(name), `${name} must appear exactly once`).toHaveLength(1);
      expect(query.get(name), `${name} must not be empty`).not.toBe("");
    }
    expect([...query.keys()].sort()).toEqual(["from", "timezone", "to"]);
    await fulfillJSON(
      route,
      statsResponse(query, {
        range: { from: "2026-08-01", to: "2026-08-08", timezone: "America/New_York" },
        interceptions: [{ kind: "future-proof", count: 7 }]
      })
    );
  });

  const initialRequests = requests.length;
  await page.locator("#stats-from").fill("2026-08-01");
  await page.locator("#stats-to").fill("2026-08-08");
  await page.locator("#stats-timezone").fill("America/New_York");
  await page.getByRole("button", { name: "更新统计" }).click();

  await expect.poll(() => requests.length).toBeGreaterThan(initialRequests);
  const latest = requests.at(-1);
  expect(latest?.get("from")).toBe("2026-08-01");
  expect(latest?.get("to")).toBe("2026-08-08");
  expect(latest?.get("timezone")).toBe("America/New_York");
  await expect(page.locator("[data-stats-interceptions-table]")).toContainText("future-proof");
  await expect(page.locator("[data-stats-results-heading]")).toContainText("America/New_York");
});

test("statistics distinguishes zero-valued days from a range without calendar days", async ({ page }) => {
  let requests = 0;

  await openLiveStats(page, async (route, query) => {
    requests += 1;
    const isEmptyRange = query.get("from") === query.get("to");
    await fulfillJSON(
      route,
      statsResponse(query, {
        summary: outcome({
          challenges: 0,
          approved: 0,
          declined: 0,
          banned: 0,
          expired: 0,
          pass_rate: 0
        }),
        trend: isEmptyRange
          ? []
          : [{ date: query.get("from") ?? "2026-09-01", ...outcome({ challenges: 0, approved: 0, declined: 0, banned: 0, expired: 0, pass_rate: 0 }) }],
        interceptions: []
      })
    );
  });

  await expect(page.getByText("所选日期范围内没有入群结果")).toBeVisible();
  await expect(page.locator("[data-stats-trend-table] tbody tr")).toHaveCount(1);
  const initialRequests = requests;
  await page.locator("#stats-from").fill("2026-09-08");
  await page.locator("#stats-to").fill("2026-09-08");
  await page.getByRole("button", { name: "更新统计" }).click();

  await expect.poll(() => requests).toBeGreaterThan(initialRequests);
  await expect(page.getByText("所选范围不包含任何日期。")).toHaveCount(2);
  await expect(page.locator("[data-stats-trend-table]")).toHaveCount(0);
});

test("statistics rejects an invalid IANA time zone before requesting the endpoint", async ({ page }) => {
  let statsRequests = 0;

  await openLiveStats(page, async (route, query) => {
    statsRequests += 1;
    await fulfillJSON(route, statsResponse(query));
  });

  const requestsBeforeValidation = statsRequests;
  await page.locator("#stats-timezone").fill("Not/A_Time_Zone");
  await page.getByRole("button", { name: "更新统计" }).click();

  await expect(page.locator("#stats-query-error")).toContainText("请输入有效的 IANA 时区名称。");
  expect(statsRequests).toBe(requestsBeforeValidation);
});
