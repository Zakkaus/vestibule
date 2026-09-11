import { expect, test, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

const selectedGroupID = "-1009000010001";
const selectedGroupTitle = "Gentoo-zh Community";
const otherGroupIDs = ["-1009000010002", "-1009000010003"] as const;
const actorID = "741928306";

type Role = "manager" | "operator";
const metricIcons = {
  challenges: "chartColumn",
  "pass-rate": "circleCheck",
  waiting: "inbox",
  banned: "shieldOff"
} as const;

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
    revision: 7,
    name_spoiler: { value: true, source: "factory default" },
    verify_max_fails: { value: 3, source: "factory default" },
    verify_retry_seconds: { value: 180, source: "factory default" },
    ban_seconds: { value: 0, source: "factory default" },
    mute_seconds: { value: 3600, source: "factory default" },
    verify_invited: { value: true, source: "factory default" },
    fallback_builtin: { value: true, source: "factory default" },
    lang: { value: "zh", source: "factory default" },
    required_channel_id: { value: 0, source: "factory default" },
    required_channel_fail_open: { value: false, source: "factory default" },
    channel_display: { value: "", source: "factory default" },
    channel_invite_url: { value: "", source: "factory default" },
    admin_log_chat_id: { value: 0, source: "factory default" },
    enabled: { value: true, source: "factory default" },
    delivery_mode: { value: "both", source: "user file" },
    verify_mode: { value: "mixed", source: "chat override" },
    timeout_seconds: { value: 300, source: "factory default" },
    questions: { value: [{ q: "Which package manager belongs to Gentoo?", options: ["Portage", "apt"], answer: 0 }, { q: "Which distribution uses ebuilds?", options: ["Debian", "Gentoo"], answer: 1 }], source: "chat override" },
    fallback_questions: { value: [{ q: "Name a Gentoo package manager", answers: ["Portage"] }], source: "user file" },
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
async function expectHomeWithinViewport(page: Page, maximumHeight: number): Promise<void> {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  });
  const geometry = await page.evaluate(() => {
    const panel = document.querySelector<HTMLElement>(".console-content")!;
    return {
      scrollHeight: document.documentElement.scrollHeight + Math.max(0, panel.scrollHeight - panel.clientHeight),
      viewportHeight: window.innerHeight
    };
  });
  expect(geometry.scrollHeight).toBeLessThanOrEqual(maximumHeight * geometry.viewportHeight);
}

async function expectRenderedHomeChart(page: Page): Promise<void> {
  const chart = page.getByTestId("home-combined-chart");
  await expect(chart).toBeVisible();
  await expect(chart.locator("svg")).toBeVisible();
  const geometry = await chart.evaluate((element) => {
    const svg = element.querySelector("svg");
    const plot = element.closest<HTMLElement>("[data-home-trend-scroll]")?.firstElementChild;
    const scroll = element.closest<HTMLElement>("[data-home-trend-scroll]");
    if (!svg || !(plot instanceof HTMLElement) || !scroll) {
      throw new Error("Home chart geometry is missing");
    }
    return {
      svgHeight: svg.getBoundingClientRect().height,
      plotHeight: plot.getBoundingClientRect().height,
      scrollHeight: scroll.scrollHeight,
      scrollClientHeight: scroll.clientHeight
    };
  });
  expect(geometry.svgHeight).toBeGreaterThan(0);
  expect(geometry.plotHeight).toBeGreaterThan(0);
  expect(geometry.svgHeight).toBeLessThanOrEqual(geometry.plotHeight + 1);
  expect(geometry.scrollHeight).toBeLessThanOrEqual(geometry.scrollClientHeight + 1);
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
        is_owner: false,
        csrf_token: "home-csrf"
      });
      return;
    }
    if (pathname === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, {
        chats: [
          { id: selectedGroupID, title: selectedGroupTitle, owner: null, administrators: [], administrators_status: "unavailable" },
          { id: otherGroupIDs[0], title: "Arch Linux Community", owner: null, administrators: [], administrators_status: "unavailable" },
          { id: otherGroupIDs[1], title: "Linux Study Group", owner: null, administrators: [], administrators_status: "unavailable" }
        ]
      });
      return;
    }
    if ([selectedGroupID, ...otherGroupIDs].some((id) => pathname.startsWith(`/api/chats/${id}/`)) && request.method() === "GET") {
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

for (const role of ["manager", "operator"] as const) {
  for (const height of [900, 720] as const) {
    test(`${role} home fits the required viewport bound at 1280x${height}`, async ({ page }) => {
      await page.setViewportSize({ width: 1280, height });
      const observations = await mockHomeTransport(page, {
        role,
        queueItems: [pendingQueueItem],
        diagnostics: role === "operator" ? {
          ...healthyDiagnostics({ writable: false }),
          health: { live: true, ready: false, config_ready: false, telegram_ready: false }
        } : undefined
      });

      await page.goto(`/home?group=${selectedGroupID}`);
      await expect(page).toHaveURL(new RegExp(`/home\\?group=${selectedGroupID}$`));
      await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
      await expectRenderedHomeChart(page);
      await expectHomeWithinViewport(page, height === 900 ? 1 : 1.15);

      await expect(page.locator("[data-home-context]")).toBeVisible();
      await expect(page.locator("[data-home-page]")).not.toContainText(/-100\d+/);
      await expect(page.locator("[data-group-switcher]")).not.toContainText(/-100\d+/);
      await expect(page.locator("[data-home-metric]")).toHaveCount(4);
      for (const [metric, icon] of Object.entries(metricIcons)) {
        await expect(page.locator(`[data-home-metric="${metric}"] [data-icon-name]`)).toHaveAttribute("data-icon-name", icon);
      }

      expect(observations.groupRequests.sort()).toEqual([
        `/api/chats/${selectedGroupID}/queue`,
        `/api/chats/${selectedGroupID}/settings`,
        `/api/chats/${selectedGroupID}/stats`
      ]);
      expect(observations.statusRequests).toBe(role === "operator" ? 1 : 0);
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

      if (role === "manager") {
        await expect(page.locator("[data-home-attention='queue']")).toBeVisible();
        await expect(page.locator("[data-home-attention='queue'] [data-home-attention-tone] [data-icon-name]")).toHaveAttribute("data-icon-name", "inbox");
      } else {
        await expect(page.locator("[data-home-attention='persistence-unwritable']")).toBeVisible();
        await expect(page.locator("[data-home-attention='persistence-unwritable'] [data-home-attention-tone] [data-icon-name]")).toHaveAttribute("data-icon-name", "circleAlert");
        await expect(page.locator("[data-home-metric='challenges']")).toContainText("70");
      }
    });
  }
}

test("home switches its context to the selected chat title without showing transport IDs", async ({ page }) => {
  await mockHomeTransport(page, { role: "manager" });
  await page.goto(`/home?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-context]")).toContainText(selectedGroupTitle);

  const switcher = page.getByRole("button", { name: "当前群组" });
  await selectAppOption(switcher, otherGroupIDs[0]);
  await expect(page).toHaveURL(new RegExp(`/home\\?group=${otherGroupIDs[0]}$`));
  await expect(page.locator("[data-home-context]")).toContainText("Arch Linux Community");
  await expect(page.locator("[data-home-context]")).not.toContainText(selectedGroupTitle);
  await expect(page.locator("[data-home-page]")).not.toContainText(/-100\d+/);
  await expect(switcher).toContainText("Arch Linux Community");
  await expect(switcher).not.toContainText(/-100\d+/);
});

test("group administrators see an explicit all-clear state without an operator status request", async ({ page }) => {
  const observations = await mockHomeTransport(page, { role: "manager" });

  await page.goto(`/home?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
  await expect(page.locator("[data-home-attention-empty]")).toBeVisible();
  await expect(page.locator("[data-home-attention-empty] [data-icon-name]")).toHaveAttribute("data-icon-name", "circleCheck");
  await expect(page.locator("[data-home-attention^='diagnostics']")).toHaveCount(0);
  expect(observations.statusRequests).toBe(0);
});

