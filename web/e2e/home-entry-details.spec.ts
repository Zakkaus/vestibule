import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1009000010001";
const actorID = "741928306";

function sourced<T>(value: T, source = "factory default") {
  return { value, source };
}

const settingsPayload = {
  revision: 12,
  enabled: sourced(true),
  delivery_mode: sourced("dm", "user file"),
  verify_mode: sourced("quiz", "chat override"),
  name_spoiler: sourced(false, "chat override"),
  timeout_seconds: sourced(420, "user file"),
  verify_max_fails: sourced(4),
  verify_retry_seconds: sourced(-1, "chat override"),
  ban_seconds: sourced(0),
  mute_seconds: sourced(7200, "user file"),
  verify_invited: sourced(false, "chat override"),
  questions: sourced([
    { q: "Which package manager belongs to Gentoo?", options: ["Portage", "apt"], answer: 0 },
    { q: "Which init system is common in Gentoo?", options: ["OpenRC", "launchd"], answer: 0 },
    { q: "Which tool builds Gentoo packages?", options: ["ebuild", "cargo"], answer: 0 }
  ], "chat override"),
  fallback_questions: sourced([
    { q: "Name a Gentoo package manager", answers: ["Portage"] },
    { q: "Name the Gentoo package tree", answers: ["ebuild"] }
  ], "user file"),
  fallback_builtin: sourced(false, "user file"),
  lang: sourced("zh-Hant", "chat override"),
  trusted_member_group_ids: sourced([-1007000000001, -1007000000002], "user file"),
  required_channel_id: sourced(-1008000000001, "chat override"),
  required_channel_fail_open: sourced(false),
  channel_display: sourced("@gentoo_required", "user file"),
  channel_invite_url: sourced("https://t.me/+gentoo-required", "chat override"),
  channel_whitelist: sourced([-1009000000001, -1009000000002, -1009000000003], "user file"),
  antispam_enabled: sourced(false, "chat override"),
  warn_limit: sourced(6, "user file"),
  admin_log_chat_id: sourced(-1009100000001, "chat override")
} as const;

const statsPayload = {
  range: { from: "2026-08-26", to: "2026-09-02", timezone: "UTC" },
  summary: { challenges: 70, approved: 41, declined: 15, banned: 4, expired: 10, pass_rate: 0.586 },
  trend: [
    { date: "2026-08-26", challenges: 8, approved: 4, declined: 4, banned: 0, expired: 0, pass_rate: 0.5 },
    { date: "2026-08-27", challenges: 12, approved: 6, declined: 6, banned: 0, expired: 0, pass_rate: 0.5 },
    { date: "2026-08-28", challenges: 7, approved: 3, declined: 4, banned: 0, expired: 0, pass_rate: 0.43 },
    { date: "2026-08-29", challenges: 15, approved: 10, declined: 5, banned: 0, expired: 0, pass_rate: 0.67 },
    { date: "2026-08-30", challenges: 9, approved: 5, declined: 4, banned: 0, expired: 0, pass_rate: 0.56 },
    { date: "2026-08-31", challenges: 11, approved: 7, declined: 4, banned: 0, expired: 0, pass_rate: 0.64 },
    { date: "2026-09-01", challenges: 8, approved: 6, declined: 2, banned: 0, expired: 0, pass_rate: 0.75 }
  ],
  interceptions: []
};

async function fulfillJSON(route: Route, body: unknown): Promise<void> {
  await route.fulfill({
    status: 200,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockHome(page: Page): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const { pathname } = url;

    if (pathname === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2026-09-02T03:00:00Z",
        csrf_token: "home-entry-details-csrf"
      });
      return;
    }
    if (pathname === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, {
        chats: [
          { id: selectedGroupID, title: "Gentoo Chinese Community" },
          { id: "-1009000010002", title: "Arch Linux Chinese Community" }
        ]
      });
      return;
    }
    if (pathname === `/api/chats/${selectedGroupID}/queue` && request.method() === "GET") {
      await fulfillJSON(route, { items: [] });
      return;
    }
    if (pathname === `/api/chats/${selectedGroupID}/stats` && request.method() === "GET") {
      await fulfillJSON(route, statsPayload);
      return;
    }
    if (pathname === `/api/chats/${selectedGroupID}/settings` && request.method() === "GET") {
      await fulfillJSON(route, settingsPayload);
      return;
    }
    if (pathname === "/api/instance" && request.method() === "GET") {
      await fulfillJSON(route, { bot_username: "example_bot" });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${pathname}`);
  });
}

test("home configuration entries stay stacked, sourced, and limited to three values", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await mockHome(page);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(`/home?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");

  const entries = page.locator("[data-home-entry]");
  await expect(entries).toHaveCount(4);
  const expected = [
    ["verification", "Multiple-choice question", "shieldCheck"],
    ["questions", "3 questions", "bookOpen"],
    ["bypass", "Configured", "usersRound"],
    ["moderation", "Disabled", "shieldAlert"]
  ] as const;

  for (const [id, value, icon] of expected) {
    const entry = page.locator(`[data-home-entry="${id}"]`);
    await expect(entry.locator("[data-icon-name]")).toHaveAttribute("data-icon-name", icon);
    const values = entry.locator("[data-home-entry-value]");
    const valueText = values.locator("[data-home-entry-value-text]");
    const sources = values.locator("[data-home-entry-source]");
    const matchingValue = valueText.filter({ hasText: value });
    const valueCount = await values.count();
    expect(valueCount).toBeGreaterThanOrEqual(1);
    expect(valueCount).toBeLessThanOrEqual(3);
    expect(await valueText.count()).toBe(valueCount);
    expect(await sources.count()).toBe(valueCount);
    await expect(matchingValue).toHaveCount(1);
    await expect(matchingValue).toHaveText(value);
    const matchingSource = matchingValue.locator("xpath=..").locator("[data-home-entry-source]");
    await expect(matchingSource).toBeVisible();
    await expect(matchingSource).toHaveText("(Group override)");
    await expect(matchingSource).toHaveAttribute("aria-label", "Source: Group override");
  }

  await expect(page.locator("body")).not.toContainText(/-100\d+/);

  const geometry = await entries.evaluateAll((elements) => ({
    available: elements[0].parentElement!.getBoundingClientRect().width,
    boxes: elements.map((element) => {
      const { top, bottom, width, height } = element.getBoundingClientRect();
      return { top, bottom, width, height };
    })
  }));
  for (const [index, box] of geometry.boxes.entries()) {
    expect(Math.abs(box.width - geometry.available)).toBeLessThanOrEqual(1);
    if (index > 0) expect(box.top).toBeGreaterThan(geometry.boxes[index - 1].bottom);
  }


  const sources = await page.locator("[data-home-entry-value]").evaluateAll((values) =>
    values.map((value) => {
      const text = value.querySelector("[data-home-entry-value-text]")!;
      const source = value.querySelector("[data-home-entry-source]")!;
      const textStyle = getComputedStyle(text);
      const sourceStyle = getComputedStyle(source);
      // Inline annotations may wrap; their union box is not a painted fragment.
      const textBox = Array.from(text.getClientRects()).at(-1)!;
      const sourceBox = source.getClientRects()[0]!;
      return {
        textSize: Number.parseFloat(textStyle.fontSize),
        sourceSize: Number.parseFloat(sourceStyle.fontSize),
        textTop: textBox.top,
        textBottom: textBox.bottom,
        textRight: textBox.right,
        sourceTop: sourceBox.top,
        sourceLeft: sourceBox.left
      };
    })
  );
  for (const source of sources) {
    expect(source.sourceSize).toBeLessThan(source.textSize);
    expect(source.sourceTop).toBeGreaterThanOrEqual(source.textTop - 2);
    if (source.sourceTop < source.textBottom) {
      expect(source.sourceLeft).toBeGreaterThanOrEqual(source.textRight - 1);
    }
  }

  await expect(page.locator("[data-home-entry='verification']")).toHaveAttribute("href", `/verification?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-entry='questions']")).toHaveAttribute("href", `/questions?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-entry='bypass']")).toHaveAttribute("href", `/bypass?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-entry='moderation']")).toHaveAttribute("href", `/moderation?group=${selectedGroupID}`);
});
