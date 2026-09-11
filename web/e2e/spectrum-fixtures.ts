import type { Page, Route } from "@playwright/test";

export const selectedGroupID = "-1009000010001";
export const actorID = "741928306";

export type SpectrumRole = "manager" | "operator";
export type TrendMode = "full" | "zero" | "gap" | "single";

export const operatorNavigationGroups = [
  { id: "daily", paths: ["/home", "/queue", "/audit"] },
  { id: "verification", paths: ["/verification", "/questions", "/bypass"] },
  { id: "group", paths: ["/groups", "/moderation", "/messages"] },
  { id: "content", paths: ["/feeds"] },
  { id: "observe", paths: ["/stats", "/diagnostics"] },
  { id: "console", paths: ["/version", "/capabilities", "/preferences"] }
] as const;

export const managerNavigationGroups = operatorNavigationGroups.map((group) => ({
  ...group,
  paths: group.paths.filter((path) => path !== "/version")
}));

export const chartDays = [
  { date: "2026-08-26", challenges: 8, passRate: 0.5 },
  { date: "2026-08-27", challenges: 12, passRate: 0.58 },
  { date: "2026-08-28", challenges: 7, passRate: 0.43 },
  { date: "2026-08-29", challenges: 15, passRate: 0.67 },
  { date: "2026-08-30", challenges: 9, passRate: 0.56 },
  { date: "2026-08-31", challenges: 11, passRate: 0.64 },
  { date: "2026-09-01", challenges: 8, passRate: 0.75 }
] as const;

const queueItems = [
  {
    id: "queue-1",
    user: "@waiting",
    group_key: selectedGroupID,
    result: { state: "pending", reason: null },
    occurred_at: "2026-09-01T01:00:00Z",
    expires_at: "2026-09-01T01:05:00Z",
    remaining_seconds: 180
  },
  {
    id: "queue-2",
    user: "@approved",
    group_key: selectedGroupID,
    result: { state: "approved", reason: null },
    occurred_at: "2026-09-01T00:45:00Z",
    expires_at: "2026-09-01T00:50:00Z",
    remaining_seconds: null
  }
] as const;

const settingsPayload = {
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
} as const;

function outcome(challenges: number, passRate: number) {
  const approved = Math.floor(challenges * passRate);
  return {
    challenges,
    approved,
    declined: challenges - approved,
    banned: 0,
    expired: 0,
    pass_rate: passRate
  };
}

function statsPayload(url: URL, mode: TrendMode) {
  const trend = (mode === "zero"
    ? chartDays.map(({ date }) => ({ date, ...outcome(0, 0) }))
    : mode === "single"
      ? chartDays.slice(-1).map(({ date, challenges, passRate }) => ({ date, ...outcome(challenges, passRate) }))
      : chartDays.map(({ date, challenges, passRate }) => ({ date, ...outcome(challenges, passRate) })))
    .filter((day) => mode !== "gap" || day.date !== "2026-08-28");
  const summary = mode === "zero"
    ? outcome(0, 0)
    : { challenges: 70, approved: 41, declined: 15, banned: 4, expired: 10, pass_rate: 0.586 };
  return {
    range: {
      from: url.searchParams.get("from") ?? chartDays[0].date,
      to: url.searchParams.get("to") ?? "2026-09-02",
      timezone: url.searchParams.get("timezone") ?? "UTC"
    },
    summary,
    trend,
    interceptions: []
  };
}

const statusPayload = {
  version: "v5.1.0",
  replacement: { unit_available: false, last_result: null },
  health: { live: true, ready: true, config_ready: true, telegram_ready: true },
  bot_api: { last_heartbeat_at: "2026-09-02T01:00:00Z", latency_ms: 18 },
  persistence: { configured: true, durable: true, writable: true, last_error: null }
} as const;

const dailyStatusPayload = {
  time: "09:00",
  timezone: "UTC"
} as const;

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

export async function installSpectrumClock(page: Page): Promise<void> {
  await page.clock.setFixedTime(new Date("2026-09-01T12:00:00Z"));
}

type MockOptions = Readonly<{
  role?: SpectrumRole;
  trend?: TrendMode;
}>;

export async function mockSpectrumTransport(page: Page, options: MockOptions = {}): Promise<void> {
  const role = options.role ?? "operator";
  const trend = options.trend ?? "full";
  let dailyEnabled = true;

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = decodeURIComponent(url.pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role },
        expires_at: "2026-09-02T03:00:00Z",
        csrf_token: "spectrum-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, {
        chats: [
          { id: selectedGroupID, title: "Gentoo-zh Community" },
          { id: "-1009000010002", title: "Arch Linux Community" }
        ]
      });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/stats` && request.method() === "GET") {
      await fulfillJSON(route, statsPayload(url, trend));
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "GET") {
      await fulfillJSON(route, settingsPayload);
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/queue` && request.method() === "GET") {
      await fulfillJSON(route, { items: queueItems });
      return;
    }
    if (path.startsWith(`/api/chats/${selectedGroupID}/queue/`) && request.method() === "POST") {
      await fulfillJSON(route, queueItems[0]);
      return;
    }
    if (path === "/api/status" && request.method() === "GET") {
      await fulfillJSON(route, statusPayload);
      return;
    }
    if (path === "/api/status/daily" && (request.method() === "GET" || request.method() === "PATCH")) {
      if (request.method() === "PATCH") {
        const payload = request.postDataJSON();
        if (
          typeof payload === "object" &&
          payload !== null &&
          !Array.isArray(payload) &&
          "enabled" in payload &&
          typeof payload.enabled === "boolean"
        ) {
          dailyEnabled = payload.enabled;
        }
      }
      await fulfillJSON(route, { enabled: dailyEnabled, ...dailyStatusPayload });
      return;
    }
    if (path === "/api/status/release" && request.method() === "GET") {
      await fulfillJSON(route, {
        version: "v5.2.0",
        url: "https://github.com/Zakkaus/vestibule/releases/tag/v5.2.0",
        notes: "Safer replacement",
        published_at: "2026-09-01T00:00:00Z",
        update_available: true,
        rollback: {
          available: true,
          reason: "",
          target_schema_version: 2,
          retained_schema_version: 2,
          minimum_rollback_schema_version: 1
        }
      });
      return;
    }
    if (path === "/api/instance" && request.method() === "GET") {
      await fulfillJSON(route, { bot_username: "example_bot" });
      return;
    }
    if (path === "/api/process/settings" && request.method() === "GET") {
      await fulfillJSON(route, {
        feeds: { value: [], source: "factory default" },
        news_url: { value: "https://example.invalid/news.xml", source: "factory default" },
        overlays: { value: [], source: "factory default" },
        stats_timezone: { value: "UTC", source: "factory default" }
      });
      return;
    }

    // Navigation coverage should exercise the shell independently of each
    // screen's domain parser. Unknown requests must fail loudly rather than
    // hiding a missing fixture behind a successful-looking response.
    await fulfillJSON(route, { error: { code: "spectrum_fixture_unavailable" } }, 503);
  });
}

export async function openSpectrumRoute(page: Page, path: string): Promise<void> {
  const url = path === "/" || path === "/version" ? path : `${path}?group=${selectedGroupID}`;
  await page.goto(url);
  await page.locator("[data-app-shell]").waitFor({ state: "visible" });
  await page.locator("[data-console-page], [data-page-heading]").first().waitFor({ state: "visible" });
  await page.evaluate(async () => {
    await document.fonts.ready;
    await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  });
}
