import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1001163306055";
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
          { id: "-1001163306066", title: "Arch Linux Chinese Community" }
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

test("home configuration entries show complete sourced settings as full-width navigable sections", async ({ page }) => {
  await mockHome(page);
  await page.goto(`/home?group=${selectedGroupID}`);
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");

  const entries = page.locator("[data-home-entry]");
  await expect(entries).toHaveCount(4);

  const verification = page.locator("[data-home-entry='verification']");
  const questions = page.locator("[data-home-entry='questions']");
  const bypass = page.locator("[data-home-entry='bypass']");
  const moderation = page.locator("[data-home-entry='moderation']");
  const values = [
    [verification, {
      "验证策略": "选择题", "投递渠道": "私聊优先", "超时": "420 秒",
      "自动封禁失败上限": "4 次失败", "重试间隔": "已停用", "拒绝后的封禁时长": "永久",
      "禁言时长": "7200 秒", "验证被邀请成员": "已停用", "隐藏申请人姓名": "已停用"
    }],
    [questions, {
      "选择题": "3 道题", "备用题": "2 道题", "备用题来源": "自定义备用题", "答题语言": "繁体中文"
    }],
    [bypass, {
      "信任群": "2 个群", "要求加入的频道": "已配置", "频道显示名": "@gentoo_required",
      "频道邀请链接": "https://t.me/+gentoo-required", "频道查询失败时放行": "已停用", "频道白名单": "3 个频道"
    }],
    [moderation, { "反垃圾": "已停用", "警告上限": "6 次警告", "处罚记录群": "已配置" }]
  ] as const;
  for (const [entry, fields] of values) {
    for (const [label, value] of Object.entries(fields)) {
      const row = entry.locator("[data-home-entry-value]").filter({ has: page.getByText(label, { exact: true }) });
      await expect(row.locator("[data-home-entry-value-text]")).toHaveText(value);
      await expect(row.locator("[data-home-entry-source]")).toBeVisible();
    }
  }
  await expect(page.locator("body")).not.toContainText(/-100\d+/);

  const geometry = await entries.evaluateAll((elements) => ({
    available: elements[0].parentElement!.getBoundingClientRect().width,
    boxes: elements.map((element) => {
      const { top, bottom, width } = element.getBoundingClientRect();
      return { top, bottom, width };
    })
  }));
  for (const [index, box] of geometry.boxes.entries()) {
    expect(Math.abs(box.width - geometry.available)).toBeLessThanOrEqual(1);
    if (index > 0) expect(box.top).toBeGreaterThan(geometry.boxes[index - 1].bottom);
  }

  const sources = await page.locator("[data-home-entry-source]").evaluateAll((elements) =>
    elements.map((element) => {
      const source = getComputedStyle(element);
      const value = getComputedStyle(element.previousElementSibling!);
      return { sourceColor: source.color, valueColor: value.color, border: source.borderStyle, background: source.backgroundColor };
    })
  );
  for (const source of sources) {
    expect(source.sourceColor).not.toBe(source.valueColor);
    expect(source.border).toBe("none");
    expect(source.background).toBe("rgba(0, 0, 0, 0)");
  }

  await expect(verification).toHaveAttribute("href", `/verification?group=${selectedGroupID}`);
  await expect(questions).toHaveAttribute("href", `/questions?group=${selectedGroupID}`);
  await expect(bypass).toHaveAttribute("href", `/bypass?group=${selectedGroupID}`);
  await expect(moderation).toHaveAttribute("href", `/moderation?group=${selectedGroupID}`);
});
