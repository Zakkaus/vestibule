import { expect, test, type Page, type Route } from "@playwright/test";

import { readRenderRoutes } from "./render-gate-routes";

const selectedGroupID = "-1001163306055";
const actorID = "741928306";

const routes = readRenderRoutes();

function sourced<T>(value: T, source = "factory default") {
  return { value, source };
}

const settingsPayload = {
  revision: 7,
  enabled: sourced(true),
  antispam_enabled: sourced(true, "user file"),
  warn_limit: sourced(3),
  admin_log_chat_id: sourced(0),
  delivery_mode: sourced("both"),
  verify_mode: sourced("kernel"),
  timeout_seconds: sourced(240, "user file"),
  verify_max_fails: sourced(3),
  verify_retry_seconds: sourced(180),
  ban_seconds: sourced(0),
  mute_seconds: sourced(3600),
  verify_invited: sourced(true),
  trusted_member_group_ids: sourced([-1007000000001]),
  required_channel_id: sourced(-1008000000001),
  required_channel_fail_open: sourced(true),
  channel_display: sourced("@required"),
  channel_invite_url: sourced(""),
  channel_whitelist: sourced([-1009000000001]),
  questions: sourced([
    {
      q: "Which package manager belongs to Gentoo?",
      options: ["Portage", "apt"],
      answer: 0
    }
  ]),
  fallback_questions: sourced([{ q: "Name a Gentoo package manager", answers: ["Portage"] }]),
  fallback_builtin: sourced(true),
  lang: sourced("zh"),
  name_spoiler: sourced(true),
  lookup_auto_delete_enabled: sourced(true),
  lookup_ttl_seconds: sourced(180),
  rich_messages: sourced(false)
} as const;

const processSettings = {
  feeds: sourced([
    {
      chat_id: -1009000000203,
      lang: "en",
      interval_seconds: 600,
      bugs: false,
      news: true,
      bug_product: "Gentoo Linux",
      bug_component: "Portage",
      silent_bugs: true
    }
  ]),
  news_url: sourced("https://example.invalid/news-items.xml"),
  overlays: sourced([{ name: "gentoo", repo: "gentoo/gentoo", branch: "master" }]),
  stats_timezone: sourced("Asia/Shanghai")
} as const;

const statusPayload = {
  version: "v5.1.0",
  replacement: { unit_available: false, last_result: null },
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
    last_error: null
  }
} as const;

function statsPayload(query: URLSearchParams) {
  return {
    range: {
      from: query.get("from") ?? "2026-09-01",
      to: query.get("to") ?? "2026-09-08",
      timezone: query.get("timezone") ?? "UTC"
    },
    summary: {
      challenges: 10,
      approved: 5,
      declined: 2,
      banned: 1,
      expired: 2,
      pass_rate: 0.5
    },
    trend: [
      {
        date: query.get("from") ?? "2026-09-01",
        challenges: 10,
        approved: 5,
        declined: 2,
        banned: 1,
        expired: 2,
        pass_rate: 0.5
      }
    ],
    interceptions: [{ kind: "kernel", count: 3 }]
  };
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockSpacingTransport(page: Page): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = decodeURIComponent(url.pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "operator" },
        expires_at: "2026-09-02T03:00:00Z",
        csrf_token: "spacing-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID }] });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "GET") {
      await fulfillJSON(route, settingsPayload);
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/stats` && request.method() === "GET") {
      await fulfillJSON(route, statsPayload(url.searchParams));
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/queue` && request.method() === "GET") {
      await fulfillJSON(route, { items: [] });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/rules` && request.method() === "GET") {
      await fulfillJSON(route, [
        {
          id: "auto-a",
          collection: "auto_reply",
          ordinal: 0,
          enabled: true,
          definition: { match: ["matrix"], reply: { text: "Bridge address" } }
        }
      ]);
      return;
    }
    if (path === "/api/process/settings" && request.method() === "GET") {
      await fulfillJSON(route, processSettings);
      return;
    }
    if (path === "/api/status" && request.method() === "GET") {
      await fulfillJSON(route, statusPayload);
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

    await fulfillJSON(route, { error: { code: "spacing_audit_unavailable" } }, 503);
  });
}

async function openSpacingRoute(page: Page, path: string): Promise<void> {
  const url =
    path === "/" || path === "/version" ? path : `${path}?group=${selectedGroupID}`;
  await page.goto(url);
  await expect(page.locator("[data-app-shell]")).toBeVisible();
  await page.locator("[data-page-heading]").waitFor({ state: "visible" });
  await page.evaluate(async () => {
    await document.fonts.ready;
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(resolve))
    );
  });
}

test("content pages use the stepped page and card hierarchy", async ({ page }) => {
  await mockSpacingTransport(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ colorScheme: "light" });

  for (const route of routes) {
    await openSpacingRoute(page, route.urlPath);
    const geometry = await page.evaluate(() => {
      const contentPage = [...document.querySelectorAll<HTMLElement>("section")].find((element) =>
        element.getAttributeNames().some((name) => name.startsWith("data-") && name.endsWith("-page"))
      );
      const heading = contentPage?.querySelector<HTMLElement>("[data-page-heading]");
      if (!contentPage || !heading) {
        throw new Error("rendered route did not expose its page and heading");
      }

      const cards = [...contentPage.querySelectorAll<HTMLElement>("[data-slot='card']")].filter(
        (card) => card.getClientRects().length > 0 && getComputedStyle(card).display !== "none"
      );
      const cardPaddingMismatches = cards
        .map((card) => {
          const style = getComputedStyle(card);
          return `${style.paddingTop} ${style.paddingRight} ${style.paddingBottom} ${style.paddingLeft}`;
        })
        .filter((padding) => padding !== "16px 24px 16px 24px");

      return {
        pageGap: getComputedStyle(contentPage).gap,
        headingGap: getComputedStyle(heading).gap,
        cardCount: cards.length,
        cardPaddingMismatches
      };
    });

    expect(geometry.pageGap, `${route.urlPath}: page heading to content`).toBe("32px");
    expect(geometry.headingGap, `${route.urlPath}: page heading copy`).toBe("8px");
    expect(geometry.cardCount, `${route.urlPath}: rendered card coverage`).toBeGreaterThan(0);
    expect(geometry.cardPaddingMismatches, `${route.urlPath}: shared card edge inset`).toEqual([]);
  }

  const stackCases = [
    {
      urlPath: "/home",
      steps: [
        ["[data-home-content]", "24px"],
        ["[data-home-section]", "16px"],
        ["[data-home-entries]", "16px"]
      ]
    },
    {
      urlPath: "/verification",
      steps: [
        ["[data-verification-form]", "24px"],
        ["[data-verification-section]", "16px"]
      ]
    },
    {
      urlPath: "/bypass",
      steps: [
        ["[data-bypass-form]", "24px"],
        ["[data-bypass-settings-card]", "16px"]
      ]
    },
    {
      urlPath: "/questions",
      steps: [
        ["[data-questions-form]", "24px"],
        ["[data-questions-section]", "16px"],
        ["[data-question-list]", "16px"]
      ]
    },
    {
      urlPath: "/capabilities",
      steps: [
        ["[data-capabilities-form]", "24px"],
        ["[data-capabilities-list]", "24px"],
        ["[data-capability-card]", "16px"]
      ]
    },
    {
      urlPath: "/feeds",
      steps: [
        ["[data-feeds-content]", "24px"],
        ["[data-feeds-section]", "16px"]
      ]
    },
    {
      urlPath: "/stats",
      steps: [
        ["[data-stats-results]", "24px"],
        ["[data-stats-chart-section]", "16px"],
        ["[data-stats-table-section]", "16px"]
      ]
    },
    {
      urlPath: "/messages",
      steps: [
        ["[data-messages-settings-form]", "24px"],
        ["[data-messages-settings-section]", "16px"]
      ]
    },
    {
      urlPath: "/diagnostics",
      steps: [
        ["[data-diagnostics-content]", "24px"],
        ["[data-diagnostics-section]", "16px"]
      ]
    },
    {
      urlPath: "/version",
      steps: [
        ["[data-version-content]", "24px"],
        ["[data-version-section]", "16px"]
      ]
    },
    { urlPath: "/groups", steps: [["[data-group-list]", "16px"]] }
  ] as const;

  for (const scenario of stackCases) {
    await openSpacingRoute(page, scenario.urlPath);
    for (const [selector, expectedGap] of scenario.steps) {
      await expect(page.locator(selector).first(), `${scenario.urlPath}: ${selector}`).toHaveCSS(
        "gap",
        expectedGap
      );
    }
  }
});

test("forms use the label, control, and error steps at desktop and mobile", async ({ page }) => {
  await mockSpacingTransport(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ colorScheme: "light" });

  const copyCases = [
    { urlPath: "/moderation", selector: "[data-setting-copy]" },
    { urlPath: "/verification", selector: "[data-verification-setting-copy]" },
    { urlPath: "/bypass", selector: "[data-setting-copy]" },
    { urlPath: "/questions", selector: "[data-question-setting-copy]" },
    { urlPath: "/messages", selector: "[data-setting-copy]" }
  ] as const;
  for (const scenario of copyCases) {
    await openSpacingRoute(page, scenario.urlPath);
    await expect(page.locator(scenario.selector).first(), `${scenario.urlPath}: field copy`).toHaveCSS(
      "gap",
      "4px"
    );
  }

  await openSpacingRoute(page, "/questions");
  await expect(page.locator("[data-question-field]").first()).toHaveCSS("gap", "8px");
  await expect(page.locator("[data-question-option-field]").first()).toHaveCSS("gap", "8px");
  await page.getByLabel("题面").first().fill("");
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(
    page.locator("[data-question-field] [data-slot='field-error']").first()
  ).toBeVisible();
  const errorGeometry = await page.evaluate(() => {
    const field = document.querySelector<HTMLElement>("[data-question-field]");
    const label = field?.querySelector<HTMLElement>("label");
    const control = field?.querySelector<HTMLElement>("[data-slot='textarea']");
    const error = field?.querySelector<HTMLElement>("[data-slot='field-error']");
    if (!field || !label || !control || !error) {
      throw new Error("questions validation fixture did not render label, control, and error");
    }

    const labelBounds = label.getBoundingClientRect();
    const controlBounds = control.getBoundingClientRect();
    const errorBounds = error.getBoundingClientRect();
    return {
      gap: getComputedStyle(field).gap,
      labelToControl: controlBounds.top - labelBounds.bottom,
      controlToError: errorBounds.top - controlBounds.bottom
    };
  });
  expect(errorGeometry.gap).toBe("8px");
  expect(errorGeometry.labelToControl).toBeCloseTo(8, 1);
  expect(errorGeometry.controlToError).toBeCloseTo(8, 1);

  await openSpacingRoute(page, "/stats");
  await expect(page.locator("[data-stats-field]").first()).toHaveCSS("gap", "8px");

  await page.setViewportSize({ width: 320, height: 900 });
  const mobileCases = [
    { urlPath: "/moderation", selector: "[data-moderation-settings-card] [data-slot='setting']" },
    { urlPath: "/verification", selector: "[data-verification-setting]" },
    { urlPath: "/bypass", selector: "[data-bypass-settings-card] [data-slot='setting']" },
    { urlPath: "/questions", selector: "[data-question-setting]" },
    { urlPath: "/messages", selector: "[data-messages-settings-section] [data-slot='setting']" }
  ] as const;
  for (const scenario of mobileCases) {
    await openSpacingRoute(page, scenario.urlPath);
    const setting = page.locator(scenario.selector).first();
    await expect(setting, `${scenario.urlPath}: stacked field layout`).toHaveCSS(
      "flex-direction",
      "column"
    );
    await expect(setting, `${scenario.urlPath}: copy to control`).toHaveCSS("gap", "8px");
  }
});
