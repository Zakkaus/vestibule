import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1001163306055";
const otherGroupIDs = ["-1001163306066", "-1001163306077"] as const;
const actorID = "741928306";

type Role = "manager" | "operator";

type HomeMockOptions = Readonly<{
  role: Role;
  queueItems?: readonly unknown[];
  diagnostics?: unknown;
}>;

type HomeObservations = {
  groupRequests: string[];
  statsQueries: URLSearchParams[];
  statusRequests: number;
};

function settingsPayload(): unknown {
  return {
    enabled: { value: true, source: "factory default" },
    delivery_mode: { value: "both", source: "user file" },
    verify_mode: { value: "mixed", source: "chat override" },
    timeout_seconds: { value: 300, source: "factory default" },
    questions: { value: [{ id: "q1" }, { id: "q2" }], source: "chat override" },
    fallback_questions: { value: [{ id: "f1" }], source: "user file" },
    trusted_member_group_ids: { value: [-1001], source: "factory default" },
    channel_whitelist: { value: [-1002, -1003], source: "chat override" },
    antispam_enabled: { value: true, source: "user file" },
    warn_limit: { value: 3, source: "factory default" }
  };
}

function statsPayload(query: URLSearchParams): unknown {
  const outcomes = [
    [8, 0.5],
    [12, 0.58],
    [7, 0.43],
    [15, 0.67],
    [9, 0.56],
    [11, 0.64],
    [8, 0.75]
  ] as const;
  const dates = [
    "2026-08-26",
    "2026-08-27",
    "2026-08-28",
    "2026-08-29",
    "2026-08-30",
    "2026-08-31",
    "2026-09-01"
  ] as const;
  return {
    range: {
      from: query.get("from"),
      to: query.get("to"),
      timezone: query.get("timezone")
    },
    summary: {
      challenges: 70,
      approved: 41,
      declined: 15,
      banned: 4,
      expired: 10,
      pass_rate: 0.586
    },
    trend: outcomes.map(([challenges, passRate], index) => ({
      date: dates[index] ?? dates[dates.length - 1],
      challenges,
      approved: Math.floor(challenges * passRate),
      declined: challenges - Math.floor(challenges * passRate),
      banned: 0,
      expired: 0,
      pass_rate: passRate
    })),
    interceptions: []
  };
}

function healthyDiagnostics(overrides: Record<string, unknown> = {}): unknown {
  return {
    health: {
      live: true,
      ready: true,
      config_ready: true,
      telegram_ready: true
    },
    bot_api: {
      last_heartbeat_at: "2026-09-02T01:00:00Z",
      latency_ms: 18
    },
    persistence: {
      configured: true,
      durable: true,
      writable: true,
      last_error: null,
      ...overrides
    }
  };
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockHomeTransport(
  page: Page,
  options: HomeMockOptions
): Promise<HomeObservations> {
  const observations: HomeObservations = {
    groupRequests: [],
    statsQueries: [],
    statusRequests: 0
  };

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const { pathname } = url;

    if (pathname === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: options.role },
        expires_at: "2026-09-02T03:00:00Z",
        csrf_token: "home-csrf"
      });
      return;
    }
    if (pathname === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, {
        chats: [selectedGroupID, ...otherGroupIDs].map((id) => ({ id }))
      });
      return;
    }
    if (pathname.startsWith(`/api/chats/${selectedGroupID}/`) && request.method() === "GET") {
      observations.groupRequests.push(pathname);
      if (pathname.endsWith("/queue")) {
        await fulfillJSON(route, { items: options.queueItems ?? [] });
        return;
      }
      if (pathname.endsWith("/stats")) {
        observations.statsQueries.push(new URLSearchParams(url.searchParams));
        await fulfillJSON(route, statsPayload(url.searchParams));
        return;
      }
      if (pathname.endsWith("/settings")) {
        await fulfillJSON(route, settingsPayload());
        return;
      }
    }
    if (pathname === "/api/status" && request.method() === "GET") {
      observations.statusRequests += 1;
      await fulfillJSON(route, options.diagnostics ?? healthyDiagnostics());
      return;
    }

    if (pathname === "/api/instance" && request.method() === "GET") {
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ bot_username: "example_bot" })
      });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${pathname}`);
  });

  return observations;
}

const pendingQueueItem = {
  id: "pending-1",
  user: "@waiting",
  group_key: selectedGroupID,
  result: { state: "pending", reason: null },
  occurred_at: "2026-09-02T01:00:00Z",
  expires_at: "2026-09-02T01:05:00Z",
  remaining_seconds: 180
};

test("authenticated home summarizes only the selected group with three group requests", async ({ page }) => {
  const observations = await mockHomeTransport(page, {
    role: "manager",
    queueItems: [pendingQueueItem]
  });

  await page.goto("/");
  await expect(page).toHaveURL(new RegExp(`/home\\?group=${selectedGroupID}$`));
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");

  expect(observations.groupRequests.sort()).toEqual([
    `/api/chats/${selectedGroupID}/queue`,
    `/api/chats/${selectedGroupID}/settings`,
    `/api/chats/${selectedGroupID}/stats`
  ]);
  expect(observations.statusRequests).toBe(0);
  expect(observations.groupRequests.some((path) => otherGroupIDs.some((id) => path.includes(id)))).toBe(false);

  expect(observations.statsQueries).toHaveLength(1);
  const query = observations.statsQueries[0];
  expect([...query.keys()].sort()).toEqual(["from", "timezone", "to"]);
  for (const name of ["from", "to", "timezone"]) {
    expect(query.getAll(name), `${name} must appear exactly once`).toHaveLength(1);
    expect(query.get(name), `${name} must not be empty`).not.toBe("");
  }
  const from = new Date(`${query.get("from")}T00:00:00Z`);
  const to = new Date(`${query.get("to")}T00:00:00Z`);
  expect((to.valueOf() - from.valueOf()) / 86_400_000).toBe(7);

  await expect(page.locator("[data-home-metric]")).toHaveCount(4);
  await expect(page.locator("[data-home-attention='queue']")).toContainText("1 份申请等待处理");
  await expect(page.locator("[data-home-trend-chart]")).toBeVisible();
  await expect(page.locator("[data-home-entry-value] [data-slot='badge']")).toHaveCount(9);
  await expect(page.locator("[data-home-entry-value] [data-slot='badge']").first()).toContainText("来源：");

  const homepageControls = page.locator(
    "[data-home-page] a, [data-home-page] button, [data-home-page] input, [data-home-page] select, [data-home-page] textarea, [data-home-page] summary, [data-home-page] [role='button']"
  );
  expect(await homepageControls.count()).toBeGreaterThan(0);
  const missingSlots = await homepageControls.evaluateAll((elements) =>
    elements
      .filter((element) => !element.hasAttribute("data-slot"))
      .map((element) => element.outerHTML)
  );
  expect(missingSlots).toEqual([]);
});

test("group administrators see an explicit all-clear state without an operator status request", async ({ page }) => {
  const observations = await mockHomeTransport(page, { role: "manager" });

  await page.goto(`/home?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
  await expect(page.locator("[data-home-attention-empty]")).toContainText(
    "等待队列为空，当前群没有需要处理的事项"
  );
  await expect(page.locator("[data-home-attention^='diagnostics']")).toHaveCount(0);
  expect(observations.statusRequests).toBe(0);
});

test("operators receive instance persistence attention without losing group data", async ({ page }) => {
  const observations = await mockHomeTransport(page, {
    role: "operator",
    diagnostics: healthyDiagnostics({ writable: false })
  });

  await page.goto(`/home?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
  await expect(page.locator("[data-home-context]")).toContainText("运维");
  await expect(page.locator("[data-home-attention='persistence-unwritable']")).toContainText(
    "设置持久化不可写"
  );
  await expect(page.locator("[data-home-metric='challenges']")).toContainText("70");
  expect(observations.statusRequests).toBe(1);
});
