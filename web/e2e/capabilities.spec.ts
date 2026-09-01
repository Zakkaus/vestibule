import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1009000000701";
const actorID = "9000000702";

type SettingSource = "factory default" | "user file" | "chat override";
type SettingsPayload = Readonly<{
  revision: number;
  enabled: Readonly<{ value: boolean; source: SettingSource }>;
  antispam_enabled: Readonly<{ value: boolean; source: SettingSource }>;
  warn_limit: Readonly<{ value: number; source: SettingSource }>;
  admin_log_chat_id: Readonly<{ value: number; source: SettingSource }>;
}>;
type ReadSettingsHandler = (route: Route, requestNumber: number) => Promise<void>;
type PatchSettingsHandler = (route: Route) => Promise<void>;

const baseSettings: SettingsPayload = {
  revision: 7,
  enabled: { value: false, source: "factory default" },
  antispam_enabled: { value: true, source: "user file" },
  warn_limit: { value: 3, source: "factory default" },
  admin_log_chat_id: { value: 0, source: "factory default" }
};

function settingsPayload(overrides: Partial<SettingsPayload> = {}): SettingsPayload {
  return { ...baseSettings, ...overrides };
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockCapabilitiesTransport(
  page: Page,
  readSettings: ReadSettingsHandler,
  patchSettings: PatchSettingsHandler = async (route) => {
    throw new Error(`Unexpected settings write: ${route.request().postData()}`);
  }
): Promise<void> {
  let readRequests = 0;
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2099-09-02T02:00:00Z",
        csrf_token: "capabilities-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID }] });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "GET") {
      readRequests += 1;
      await readSettings(route, readRequests);
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "PATCH") {
      await patchSettings(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openCapabilities(
  page: Page,
  readSettings: ReadSettingsHandler,
  patchSettings?: PatchSettingsHandler
): Promise<void> {
  await mockCapabilitiesTransport(page, readSettings, patchSettings);
  await page.goto(`/capabilities?group=${selectedGroupID}`);
  await expect(page.locator("[data-capabilities-page]")).toHaveAttribute(
    "data-capabilities-state",
    "loaded"
  );
}

test("capabilities explains both states, shows provenance, and keeps one writer per setting", async ({
  page
}) => {
  await openCapabilities(page, async (route) => {
    await fulfillJSON(
      route,
      settingsPayload({ enabled: { value: false, source: "chat override" } })
    );
  });

  const verification = page.locator('[data-capability-card="verification"]');
  await expect(verification).toContainText("来源：此群覆盖");
  await expect(verification).toContainText("已关闭");
  await expect(verification).toContainText(
    "新的入群申请会进入自动验证；已直接入群且符合条件的新成员也会收到挑战。"
  );
  await expect(verification).toContainText(
    "新的入群申请留给管理员手动处理，新入群成员不会收到挑战；已经开始的验证仍会继续。"
  );
  await expect(verification.getByRole("switch", { name: "自动入群验证" })).toHaveAttribute(
    "aria-checked",
    "false"
  );
  await expect(verification.getByRole("button", { name: "恢复继承值" })).toBeVisible();

  const antispam = page.locator('[data-capability-card="antispam"]');
  await expect(antispam).toContainText("来源：配置文件");
  await expect(antispam).toContainText("已开启");
  await expect(antispam).toContainText(
    "BotFather 隐私模式关闭时，未受信任的频道身份发言会被删除；确认它不是本群关联频道后，该频道身份会被封禁，并向处罚记录群发出提醒。"
  );
  await expect(antispam).toContainText("机器人不检查频道身份，相关消息会继续交给后续处理。");
  await expect(antispam.getByRole("switch")).toHaveCount(0);
  await expect(antispam.getByRole("link", { name: "在管理与处罚中调整" })).toHaveAttribute(
    "href",
    `/moderation?group=${selectedGroupID}`
  );

});

test("capabilities uses shared controls and the established setting-screen geometry", async ({
  page
}) => {
  await openCapabilities(page, async (route) => fulfillJSON(route, settingsPayload()));

  const interactive = page.locator(
    '[data-capabilities-page] a, [data-capabilities-page] button, [data-capabilities-page] input, [data-capabilities-page] select, [data-capabilities-page] textarea'
  );
  expect(await interactive.count(), "the slot scan must inspect rendered controls").toBeGreaterThan(0);
  await expect(
    page.locator(
      '[data-capabilities-page] a:not([data-slot]), [data-capabilities-page] button:not([data-slot]), [data-capabilities-page] input:not([data-slot]), [data-capabilities-page] select:not([data-slot]), [data-capabilities-page] textarea:not([data-slot])'
    ),
    "every interactive element on the screen must use a shared slot"
  ).toHaveCount(0);

  const capabilityGeometry = await page.evaluate(() => {
    const gapSelectors = [
      "[data-capabilities-page]",
      "[data-capabilities-form]",
      "[data-capabilities-list]"
    ];
    const gaps = gapSelectors.map((selector) =>
      getComputedStyle(document.querySelector<HTMLElement>(selector)!).gap
    );
    const cards = [...document.querySelectorAll<HTMLElement>("[data-capability-card]")];
    const radii = cards.map((card) => getComputedStyle(card).borderRadius);
    const bounds = cards.map((card) => card.getBoundingClientRect());
    return {
      gaps,
      radii,
      renderedGap: bounds[1]!.top - bounds[0]!.bottom
    };
  });
  expect(new Set(capabilityGeometry.gaps).size, "all capability levels use one gap").toBe(1);
  expect(capabilityGeometry.gaps[0]).not.toBe("normal");
  expect(capabilityGeometry.renderedGap).toBeCloseTo(
    Number.parseFloat(capabilityGeometry.gaps[0]!),
    1
  );
  expect(new Set(capabilityGeometry.radii).size, "capability cards share one radius").toBe(1);

  await page.getByRole("link", { name: "在管理与处罚中调整" }).click();
  await expect(page.locator("[data-moderation-page]")).toHaveAttribute(
    "data-moderation-state",
    "loaded"
  );
  const establishedGeometry = await page.evaluate(() => ({
    gaps: ["[data-moderation-page]", "[data-moderation-form]"].map((selector) =>
      getComputedStyle(document.querySelector<HTMLElement>(selector)!).gap
    ),
    radius: getComputedStyle(
      document.querySelector<HTMLElement>("[data-moderation-settings-card]")!
    ).borderRadius
  }));
  expect(
    new Set([...capabilityGeometry.gaps, ...establishedGeometry.gaps]).size,
    "capability spacing matches an established setting screen"
  ).toBe(1);
  expect(
    new Set([...capabilityGeometry.radii, establishedGeometry.radius]).size,
    "capability card radius matches an established setting screen"
  ).toBe(1);
});

test("capabilities saves one sparse change once and preserves CSRF and revision", async ({ page }) => {
  let markPatchRequested!: () => void;
  let releasePatch!: () => void;
  const patchRequested = new Promise<void>((resolve) => {
    markPatchRequested = resolve;
  });
  const patchResponse = new Promise<void>((resolve) => {
    releasePatch = resolve;
  });
  let patchCalls = 0;
  let requestBody: unknown;
  let csrfHeader: string | undefined;

  await openCapabilities(
    page,
    async (route) => fulfillJSON(route, settingsPayload()),
    async (route) => {
      patchCalls += 1;
      requestBody = route.request().postDataJSON();
      csrfHeader = route.request().headers()["x-csrf-token"];
      markPatchRequested();
      await patchResponse;
      await fulfillJSON(
        route,
        settingsPayload({
          revision: 8,
          enabled: { value: true, source: "chat override" }
        })
      );
    }
  );

  await page.getByRole("switch", { name: "自动入群验证" }).click();
  const save = page.getByRole("button", { name: "保存更改" });
  await save.click();
  await patchRequested;
  await expect(page.getByRole("button", { name: "正在保存…" })).toHaveAttribute(
    "aria-disabled",
    "true"
  );
  await page.getByRole("button", { name: "正在保存…" }).dispatchEvent("click");
  expect(patchCalls).toBe(1);
  expect(csrfHeader).toBe("capabilities-csrf");
  expect(requestBody).toEqual({ expected_revision: 7, changes: { enabled: true } });

  releasePatch();
  await expect(page.locator('[data-capabilities-feedback="saved"]')).toContainText(
    "功能设置已保存。"
  );
  await expect(page.getByRole("switch", { name: "自动入群验证" })).toHaveAttribute(
    "aria-checked",
    "true"
  );
  await expect(page.locator('[data-capability-card="verification"]')).toContainText(
    "来源：此群覆盖"
  );
});

test("capabilities restores the editable override with an explicit null", async ({ page }) => {
  let requestBody: unknown;
  await openCapabilities(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsPayload({ enabled: { value: true, source: "chat override" } })
      ),
    async (route) => {
      requestBody = route.request().postDataJSON();
      await fulfillJSON(
        route,
        settingsPayload({
          revision: 8,
          enabled: { value: false, source: "factory default" }
        })
      );
    }
  );

  await page.getByRole("button", { name: "恢复继承值" }).click();
  await expect(page.locator("[data-capability-pending]")).toContainText("待恢复继承值");
  await expect(page.getByRole("switch", { name: "自动入群验证" })).toHaveAttribute(
    "aria-disabled",
    "true"
  );
  await page.getByRole("button", { name: "保存更改" }).click();

  expect(requestBody).toEqual({ expected_revision: 7, changes: { enabled: null } });
  await expect(page.locator('[data-capability-card="verification"]')).toContainText(
    "来源：出厂默认"
  );
  await expect(page.getByRole("button", { name: "恢复继承值" })).toHaveCount(0);
});

test("capabilities names a revision conflict and reloads the latest value", async ({ page }) => {
  let requestBody: unknown;
  await openCapabilities(
    page,
    async (route, requestNumber) => {
      await fulfillJSON(
        route,
        settingsPayload({
          revision: requestNumber === 1 ? 7 : 8,
          enabled: {
            value: requestNumber !== 1,
            source: requestNumber === 1 ? "factory default" : "chat override"
          }
        })
      );
    },
    async (route) => {
      requestBody = route.request().postDataJSON();
      await fulfillJSON(route, { error: { code: "settings_conflict" } }, 409);
    }
  );

  await page.getByRole("switch", { name: "自动入群验证" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(page.locator('[data-capabilities-feedback="conflict"]')).toContainText(
    "其他管理员已经更改功能设置。重新载入最新值后再操作。"
  );
  expect(requestBody).toEqual({ expected_revision: 7, changes: { enabled: true } });

  await page.getByRole("button", { name: "重新载入" }).click();
  await expect(page.getByRole("switch", { name: "自动入群验证" })).toHaveAttribute(
    "aria-checked",
    "true"
  );
  await expect(page.locator('[data-capabilities-feedback="conflict"]')).toHaveCount(0);
  await expect(page.locator('[data-capability-card="verification"]')).toContainText(
    "来源：此群覆盖"
  );
});
