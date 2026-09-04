import { expect, test, type Page, type Route } from "@playwright/test";

const actorID = "9000000911";
const groupID = "-1009000000912";

const settings = {
  revision: 7,
  name_spoiler: { value: true, source: "factory default" },
  lookup_auto_delete_enabled: { value: true, source: "factory default" },
  lookup_ttl_seconds: { value: 180, source: "factory default" },
  rich_messages: { value: false, source: "factory default" }
} as const;

const rules = [
  {
    id: "auto-a",
    collection: "auto_reply",
    ordinal: 0,
    enabled: true,
    definition: { match: ["matrix"], reply: { text: "Bridge address" } }
  }
] as const;

type Handler = (route: Route) => Promise<void>;

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockMessagesTransport(
  page: Page,
  readSettings: Handler,
  patchSettings: Handler,
  readRules: Handler,
  writeRules: Handler
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);
    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2026-09-02T12:00:00Z",
        csrf_token: "messages-errors-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: groupID }] });
      return;
    }
    if (path === `/api/chats/${groupID}/settings`) {
      await (request.method() === "GET" ? readSettings(route) : patchSettings(route));
      return;
    }
    if (path === `/api/chats/${groupID}/rules` && request.method() === "GET") {
      await readRules(route);
      return;
    }
    if (path.startsWith(`/api/chats/${groupID}/rules/`) && request.method() === "PUT") {
      await writeRules(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openMessages(page: Page): Promise<void> {
  await page.goto(`/messages?group=${groupID}`);
  await expect(page.locator("[data-messages-settings-form]")).toBeVisible();
  await expect(page.locator("[data-messages-rules-section]")).toBeVisible();
}

test("a missing message rule provides the reload action named by its error", async ({ page }) => {
  let ruleReads = 0;
  await mockMessagesTransport(
    page,
    async (route) => fulfillJSON(route, settings),
    async () => {
      throw new Error("A rule error must not write message settings");
    },
    async (route) => {
      ruleReads += 1;
      await fulfillJSON(route, { items: rules });
    },
    async (route) => fulfillJSON(route, { error: { code: "rule_not_found" } }, 404)
  );
  await openMessages(page);

  await page.locator("[data-messages-rule-item]").getByRole("switch").click();
  const feedback = page.locator("[data-messages-rules-feedback]");
  await expect(feedback).toContainText("这条规则已不存在。请重新读取规则。");
  await feedback.getByRole("button", { name: "重新读取" }).click();
  await expect.poll(() => ruleReads).toBe(2);
  await expect(feedback).toHaveCount(0);
});

test("an interrupted message-settings save provides a reload action", async ({ page }) => {
  let settingsReads = 0;
  await mockMessagesTransport(
    page,
    async (route) => {
      settingsReads += 1;
      await fulfillJSON(route, settings);
    },
    async (route) => route.abort("failed"),
    async (route) => fulfillJSON(route, { items: rules }),
    async () => {
      throw new Error("A settings error must not write message rules");
    }
  );
  await openMessages(page);

  await page.locator("#messages-name-spoiler").click();
  await page.getByRole("button", { name: "保存消息设置" }).click();
  const feedback = page.locator("[data-messages-settings-feedback]");
  await expect(feedback).toContainText("连接已中断");
  await feedback.getByRole("button", { name: "重新读取" }).click();
  await expect.poll(() => settingsReads).toBe(2);
  await expect(feedback).toHaveCount(0);
});

test("a rules read failure marks the messages page unavailable", async ({ page }) => {
  await mockMessagesTransport(
    page,
    async (route) => fulfillJSON(route, settings),
    async () => {
      throw new Error("A read failure must not write message settings");
    },
    async (route) => fulfillJSON(route, { error: { code: "rules_unavailable" } }, 503),
    async () => {
      throw new Error("A read failure must not write message rules");
    }
  );

  await page.goto(`/messages?group=${groupID}`);
  await expect(page.locator('[data-messages-state-card="rules-unavailable"]')).toBeVisible();
  await expect(page.locator("[data-messages-page]")).toHaveAttribute(
    "data-messages-state",
    "unavailable"
  );
});
