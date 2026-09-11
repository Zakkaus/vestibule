import { expect, type Page, type Route } from "@playwright/test";

const actorID = "9000000301";
export const selectedGroupID = "-1009000000302";
type ConsoleRole = "manager" | "operator";

type DiagnosticsHandler = (route: Route) => Promise<void>;
type DailyHandler = (route: Route) => Promise<void>;

export async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

export async function mockDiagnosticsTransport(
  page: Page,
  role: ConsoleRole,
  diagnostics: DiagnosticsHandler,
  daily: DailyHandler = async (route) => {
    const request = route.request();
    if (request.method() === "PATCH") {
      const payload = request.postDataJSON();
      if (
        typeof payload === "object" &&
        payload !== null &&
        !Array.isArray(payload) &&
        "enabled" in payload &&
        typeof payload.enabled === "boolean"
      ) {
        await fulfillJSON(route, {
          enabled: payload.enabled,
          time: "09:00",
          timezone: "Asia/Shanghai"
        });
        return;
      }
    }
    await fulfillJSON(route, { enabled: true, time: "09:00", timezone: "Asia/Shanghai" });
  }
): Promise<string[]> {
  const statusMethods: string[] = [];
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role },
        expires_at: "2026-09-02T12:00:00Z",
        csrf_token: "diagnostics-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID, title: "Gentoo-zh Community" }] });
      return;
    }
    if (path === "/api/status/daily") {
      await daily(route);
      return;
    }
    if (path === "/api/status") {
      statusMethods.push(request.method());
      await diagnostics(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
  return statusMethods;
}

export async function clickSidebarLink(page: Page, path: "/groups" | "/diagnostics"): Promise<void> {
  const group = path === "/groups" ? "group" : "observe";
  const trigger = page.locator(`.console-sidebar [data-navigation-group="${group}"]`);
  if (await trigger.getAttribute("aria-expanded") === "false") await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(page.locator(`.console-sidebar [data-navigation-items="${group}"]`))
    .toHaveAttribute("aria-hidden", "false");
  await page.locator(`.console-sidebar [data-navigation-item="${path}"]`).click();
}

export const unmeasuredDiagnostics = {
  health: {
    live: true,
    ready: false,
    config_ready: true,
    telegram_ready: false
  },
  bot_api: {
    last_heartbeat_at: null,
    latency_ms: 0
  },
  persistence: {
    configured: true,
    durable: true,
    writable: true,
    last_error: null
  }
} as const;
