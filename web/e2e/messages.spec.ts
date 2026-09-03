import { expect, test, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

const selectedGroupID = "-1001163306055";
const otherGroupID = "-1009000000004";
const actorID = "741928306";

type SettingSource = "factory default" | "user file" | "chat override";

type SourcedSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

type SettingsResponse = Readonly<{
  revision: number;
  name_spoiler: SourcedSetting<boolean>;
  lookup_auto_delete_enabled: SourcedSetting<boolean>;
  lookup_ttl_seconds: SourcedSetting<number>;
  rich_messages: SourcedSetting<boolean>;
}>;

type RuleResponse = Readonly<{
  id: string;
  collection: string;
  ordinal: number;
  enabled: boolean;
  definition: unknown;
}>;

type SettingsReadHandler = (route: Route) => Promise<void>;
type SettingsPatchHandler = (route: Route) => Promise<void>;
type RulesReadHandler = (route: Route) => Promise<void>;
type RulesWriteHandler = (route: Route) => Promise<void>;

type SettingsOverrides = Partial<SettingsResponse>;

function sourced<T>(value: T, source: SettingSource = "factory default"): SourcedSetting<T> {
  return { value, source };
}

function settingsResponse(overrides: SettingsOverrides = {}): SettingsResponse {
  return {
    revision: 7,
    name_spoiler: sourced(true),
    lookup_auto_delete_enabled: sourced(true),
    lookup_ttl_seconds: sourced(180, "user file"),
    rich_messages: sourced(false),
    ...overrides
  };
}

function initialRules(): readonly RuleResponse[] {
  return [
    {
      id: "auto-a",
      collection: "auto_reply",
      ordinal: 0,
      enabled: true,
      definition: { match: ["matrix"], reply: { text: "Bridge address" } }
    },
    {
      id: "auto-b",
      collection: "auto_reply",
      ordinal: 1,
      enabled: true,
      definition: ["bridge", { reply: "Use Matrix" }]
    },
    {
      id: "future-a",
      collection: "future_collection",
      ordinal: 0,
      enabled: true,
      definition: { future: true }
    }
  ];
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockMessagesTransport(
  page: Page,
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler,
  readRules: RulesReadHandler,
  writeRules: RulesWriteHandler
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2026-09-01T02:00:00Z",
        csrf_token: "messages-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID }, { id: otherGroupID }] });
      return;
    }
    if (
      (path === `/api/chats/${selectedGroupID}/settings` ||
        path === `/api/chats/${otherGroupID}/settings`) &&
      request.method() === "GET"
    ) {
      await readSettings(route);
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "PATCH") {
      await patchSettings(route);
      return;
    }
    if (
      (path === `/api/chats/${selectedGroupID}/rules` ||
        path === `/api/chats/${otherGroupID}/rules`) &&
      request.method() === "GET"
    ) {
      await readRules(route);
      return;
    }
    if (
      (path === `/api/chats/${selectedGroupID}/rules` ||
        path.startsWith(`/api/chats/${selectedGroupID}/rules/`)) &&
      request.method() === "PUT"
    ) {
      await writeRules(route);
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openLiveMessages(
  page: Page,
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler,
  readRules: RulesReadHandler,
  writeRules: RulesWriteHandler
): Promise<void> {
  await mockMessagesTransport(page, readSettings, patchSettings, readRules, writeRules);
  await page.goto(`/messages?group=${selectedGroupID}`);
  await expect(page.locator("[data-messages-settings-form]")).toBeVisible();
  await expect(page.locator("[data-messages-rules-section]")).toBeVisible();
}

test("messages saves a sparse settings patch separately from rules", async ({ page }) => {
  let resolvePatchSettled!: () => void;
  const patchSettled = new Promise<void>((resolve) => {
    resolvePatchSettled = resolve;
  });

  await openLiveMessages(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async (route) => {
      const request = route.request();
      expect(request.headers()["x-csrf-token"]).toBe("messages-csrf");
      expect(JSON.parse(request.postData() ?? "{}")).toEqual({
        expected_revision: 7,
        changes: { name_spoiler: false }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 8,
          name_spoiler: sourced(false, "chat override")
        })
      );
      resolvePatchSettled();
    },
    async (route) => {
      await fulfillJSON(route, { items: initialRules() });
    },
    async () => {
      throw new Error("A settings save must not write rules");
    }
  );

  await expect(page.getByText("来源：配置文件")).toBeVisible();
  await expect(page.locator("#messages-auto-delete-delay")).toBeEditable();

  await page.locator("#messages-name-spoiler").click();
  await page.locator("[data-messages-settings-savebar] [data-slot='button']").click();
  await patchSettled;
  await expect(page.locator("#messages-name-spoiler")).toHaveAttribute("aria-checked", "false");
  await expect(page.locator("[data-messages-settings-feedback]")).toHaveAttribute("data-tone", "ok");
});

test("messages toggles one opaque rule without changing an unknown collection", async ({ page }) => {
  const rules = initialRules();
  let resolveRuleSettled!: () => void;
  const ruleSettled = new Promise<void>((resolve) => {
    resolveRuleSettled = resolve;
  });

  await openLiveMessages(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async () => {
      throw new Error("Toggling a rule must not patch settings");
    },
    async (route) => {
      await fulfillJSON(route, { items: rules });
    },
    async (route) => {
      const request = route.request();
      expect(new URL(request.url()).pathname).toBe(`/api/chats/${selectedGroupID}/rules/auto-a`);
      expect(JSON.parse(request.postData() ?? "{}")).toEqual({
        expected: {
          collection: "auto_reply",
          ordinal: 0,
          enabled: true,
          definition: rules[0].definition
        },
        item: {
          collection: "auto_reply",
          ordinal: 0,
          enabled: false,
          definition: rules[0].definition
        }
      });
      await fulfillJSON(route, { ...rules[0], enabled: false });
      resolveRuleSettled();
    }
  );

  await expect(page.getByText("未知规则集合：future_collection")).toBeVisible();
  await page.locator("[data-messages-rule-item]").first().getByRole("switch").click();
  await ruleSettled;
  await expect(page.locator("[data-messages-rule-item]").first().getByRole("switch")).toHaveAttribute(
    "aria-checked",
    "false"
  );
  await expect(page.getByText("未知规则集合：future_collection")).toBeVisible();
});

test("messages reorders one collection through a complete replacement", async ({ page }) => {
  const rules = initialRules();
  let resolveRuleSettled!: () => void;
  const ruleSettled = new Promise<void>((resolve) => {
    resolveRuleSettled = resolve;
  });

  await openLiveMessages(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async () => {
      throw new Error("Reordering a collection must not patch settings");
    },
    async (route) => {
      await fulfillJSON(route, { items: rules });
    },
    async (route) => {
      const request = route.request();
      expect(new URL(request.url()).pathname).toBe(`/api/chats/${selectedGroupID}/rules`);
      expect(JSON.parse(request.postData() ?? "{}")).toEqual({
        collection: "auto_reply",
        expected: [
          { id: "auto-a", enabled: true, definition: rules[0].definition },
          { id: "auto-b", enabled: true, definition: rules[1].definition }
        ],
        items: [
          { id: "auto-b", enabled: true, definition: rules[1].definition },
          { id: "auto-a", enabled: true, definition: rules[0].definition }
        ]
      });
      await fulfillJSON(route, { items: [rules[1], rules[0]] });
      resolveRuleSettled();
    }
  );

  await page.getByRole("button", { name: "将规则 auto-b 上移" }).click();
  await ruleSettled;
  await expect(page.locator("[data-messages-rule-item]").first()).toContainText("ID：auto-b");
  await expect(page.getByText("未知规则集合：future_collection")).toBeVisible();
});

test("messages reloads settings after a revision conflict", async ({ page }) => {
  let readCount = 0;

  await openLiveMessages(
    page,
    async (route) => {
      readCount += 1;
      await fulfillJSON(
        route,
        settingsResponse({
          revision: readCount === 1 ? 7 : 8,
          name_spoiler: sourced(readCount === 1, "chat override")
        })
      );
    },
    async (route) => {
      expect(JSON.parse(route.request().postData() ?? "{}")).toEqual({
        expected_revision: 7,
        changes: { name_spoiler: false }
      });
      await fulfillJSON(route, { error: { code: "settings_conflict" } }, 409);
    },
    async (route) => {
      await fulfillJSON(route, { items: initialRules() });
    },
    async () => {
      throw new Error("A settings conflict must not write rules");
    }
  );

  await page.locator("#messages-name-spoiler").click();
  await page.locator("[data-messages-settings-savebar] [data-slot='button']").click();
  await expect(page.locator("[data-messages-settings-feedback]")).toHaveAttribute("data-tone", "error");
  await expect(page.locator("#messages-name-spoiler")).toHaveAttribute("aria-checked", "false");
  await expect.poll(() => readCount).toBe(2);
});

function otherGroupRules(): readonly RuleResponse[] {
  return [
    {
      id: "b-rule",
      collection: "auto_reply",
      ordinal: 0,
      enabled: false,
      definition: { match: ["group-b"], reply: { text: "Group B reply" } }
    }
  ];
}

test("messages settings discard a previous group's delayed response", async ({ page }) => {
  let markSettingsRequested!: () => void;
  let releaseSettings!: () => void;
  const settingsRequested = new Promise<void>((resolve) => {
    markSettingsRequested = resolve;
  });
  const firstSettingsResponse = new Promise<void>((resolve) => {
    releaseSettings = resolve;
  });
  let markSettingsResponseSettled!: () => void;
  const settingsResponseSettled = new Promise<void>((resolve) => {
    markSettingsResponseSettled = resolve;
  });

  await mockMessagesTransport(
    page,
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === `/api/chats/${selectedGroupID}/settings`) {
        markSettingsRequested();
        await firstSettingsResponse;
        await fulfillJSON(route, settingsResponse({ name_spoiler: sourced(true) }));
        markSettingsResponseSettled();
        return;
      }
      await fulfillJSON(
        route,
        settingsResponse({ revision: 8, name_spoiler: sourced(false, "chat override") })
      );
    },
    async () => {
      throw new Error("A delayed settings read must not write messages settings");
    },
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      await fulfillJSON(route, { items: path.includes(otherGroupID) ? otherGroupRules() : initialRules() });
    },
    async () => {
      throw new Error("A delayed settings read must not write message rules");
    }
  );

  await page.goto(`/messages?group=${selectedGroupID}`, { waitUntil: "domcontentloaded" });
  await settingsRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), otherGroupID);
  await expect(page).toHaveURL(new RegExp(`/messages\\?group=${otherGroupID}$`));
  await expect(page.locator("#messages-name-spoiler")).toHaveAttribute("aria-checked", "false");

  releaseSettings();
  await settingsResponseSettled;
  await expect(page.locator("#messages-name-spoiler")).toHaveAttribute("aria-checked", "false");
});

test("messages settings ignore a previous group's delayed save", async ({ page }) => {
  let markPatchRequested!: () => void;
  let releasePatch!: () => void;
  const patchRequested = new Promise<void>((resolve) => {
    markPatchRequested = resolve;
  });
  const patchResponse = new Promise<void>((resolve) => {
    releasePatch = resolve;
  });
  let markPatchResponseSettled!: () => void;
  const patchResponseSettled = new Promise<void>((resolve) => {
    markPatchResponseSettled = resolve;
  });

  await openLiveMessages(
    page,
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      await fulfillJSON(
        route,
        path === `/api/chats/${otherGroupID}/settings`
          ? settingsResponse({ revision: 8, name_spoiler: sourced(true, "chat override") })
          : settingsResponse()
      );
    },
    async (route) => {
      markPatchRequested();
      await patchResponse;
      await fulfillJSON(
        route,
        settingsResponse({ revision: 8, name_spoiler: sourced(false, "chat override") })
      );
      markPatchResponseSettled();
    },
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      await fulfillJSON(route, { items: path.includes(otherGroupID) ? otherGroupRules() : initialRules() });
    },
    async () => {
      throw new Error("A settings save must not write message rules");
    }
  );

  await page.locator("#messages-name-spoiler").click();
  await page.locator("[data-messages-settings-savebar] [data-slot='button']").click();
  await patchRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), otherGroupID);
  await expect(page).toHaveURL(new RegExp(`/messages\\?group=${otherGroupID}$`));
  await expect(page.locator("#messages-name-spoiler")).toHaveAttribute("aria-checked", "true");

  releasePatch();
  await patchResponseSettled;
  await expect(page.locator("#messages-name-spoiler")).toHaveAttribute("aria-checked", "true");
});

test("message rules discard a previous group's delayed response", async ({ page }) => {
  let markRulesRequested!: () => void;
  let releaseRules!: () => void;
  const rulesRequested = new Promise<void>((resolve) => {
    markRulesRequested = resolve;
  });
  const firstRulesResponse = new Promise<void>((resolve) => {
    releaseRules = resolve;
  });
  let markRulesResponseSettled!: () => void;
  const rulesResponseSettled = new Promise<void>((resolve) => {
    markRulesResponseSettled = resolve;
  });

  await mockMessagesTransport(
    page,
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      await fulfillJSON(
        route,
        path.includes(otherGroupID)
          ? settingsResponse({ revision: 8, name_spoiler: sourced(false, "chat override") })
          : settingsResponse()
      );
    },
    async () => {
      throw new Error("A delayed rules read must not write messages settings");
    },
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === `/api/chats/${selectedGroupID}/rules`) {
        markRulesRequested();
        await firstRulesResponse;
        await fulfillJSON(route, { items: initialRules() });
        markRulesResponseSettled();
        return;
      }
      await fulfillJSON(route, { items: otherGroupRules() });
    },
    async () => {
      throw new Error("A delayed rules read must not write message rules");
    }
  );

  await page.goto(`/messages?group=${selectedGroupID}`, { waitUntil: "domcontentloaded" });
  await rulesRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), otherGroupID);
  await expect(page).toHaveURL(new RegExp(`/messages\\?group=${otherGroupID}$`));
  await expect(page.locator("[data-messages-rule-item]").first()).toContainText("ID：b-rule");

  releaseRules();
  await rulesResponseSettled;
  await expect(page.locator("[data-messages-rule-item]").first()).toContainText("ID：b-rule");
});

test("message rules ignore a previous group's delayed save", async ({ page }) => {
  const rules = initialRules();
  let markWriteRequested!: () => void;
  let releaseWrite!: () => void;
  const writeRequested = new Promise<void>((resolve) => {
    markWriteRequested = resolve;
  });
  const writeResponse = new Promise<void>((resolve) => {
    releaseWrite = resolve;
  });
  let markWriteResponseSettled!: () => void;
  const writeResponseSettled = new Promise<void>((resolve) => {
    markWriteResponseSettled = resolve;
  });

  await openLiveMessages(
    page,
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      await fulfillJSON(
        route,
        path.includes(otherGroupID)
          ? settingsResponse({ revision: 8, name_spoiler: sourced(false, "chat override") })
          : settingsResponse()
      );
    },
    async () => {
      throw new Error("A rules save must not write messages settings");
    },
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      await fulfillJSON(route, { items: path.includes(otherGroupID) ? otherGroupRules() : rules });
    },
    async (route) => {
      markWriteRequested();
      await writeResponse;
      await fulfillJSON(route, { ...rules[0], enabled: false });
      markWriteResponseSettled();
    }
  );

  await page.locator("[data-messages-rule-item]").first().getByRole("switch").click();
  await writeRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), otherGroupID);
  await expect(page).toHaveURL(new RegExp(`/messages\\?group=${otherGroupID}$`));
  await expect(page.locator("[data-messages-rule-item]").first()).toContainText("ID：b-rule");

  releaseWrite();
  await writeResponseSettled;
  await expect(page.locator("[data-messages-rule-item]").first()).toContainText("ID：b-rule");
});
