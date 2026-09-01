import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1001163306055";
const actorID = "741928306";

const baseSettings = {
  revision: 7,
  warn_limit: { value: 3, source: "factory default" },
  antispam_enabled: { value: true, source: "factory default" },
  admin_log_chat_id: { value: 0, source: "factory default" }
} as const;

type SettingsPayload = {
  revision: number;
  warn_limit: { value: number; source: string };
  antispam_enabled: { value: boolean; source: string };
  admin_log_chat_id: { value: number; source: string };
};

type ReadSettingsHandler = (route: Route, requestNumber: number) => Promise<void>;
type PatchSettingsHandler = (route: Route) => Promise<void>;

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

async function mockModerationTransport(
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
        expires_at: "2026-09-01T02:00:00Z",
        csrf_token: "moderation-csrf"
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

async function openModeration(
  page: Page,
  readSettings: ReadSettingsHandler,
  patchSettings?: PatchSettingsHandler
): Promise<void> {
  await mockModerationTransport(page, readSettings, patchSettings);
  await page.goto(`/moderation?group=${selectedGroupID}`);
  await expect(page.locator("[data-moderation-page]")).toHaveAttribute(
    "data-moderation-state",
    "loaded"
  );
}

test("moderation shows provenance and expresses group ID zero as a blank destination", async ({
  page
}) => {
  await openModeration(page, async (route) => {
    await fulfillJSON(
      route,
      settingsPayload({
        warn_limit: { value: 5, source: "chat override" },
        admin_log_chat_id: { value: 0, source: "user file" }
      })
    );
  });

  const warningRow = page.locator('[data-moderation-field="warnLimit"]');
  await expect(warningRow).toContainText("本群设定");
  await expect(warningRow.getByRole("button", { name: "恢复默认" })).toBeVisible();

  const antispamRow = page.locator('[data-moderation-field="antispamEnabled"]');
  await expect(antispamRow).toContainText("出厂默认");
  await expect(antispamRow.getByRole("switch")).toHaveAttribute("aria-checked", "true");

  const adminLogRow = page.locator('[data-moderation-field="adminLogChatID"]');
  const adminLogInput = adminLogRow.getByLabel("处罚记录群");
  await expect(adminLogRow).toContainText("由文件管理");
  await expect(adminLogInput).toHaveValue("");
  await expect(adminLogInput).toHaveAttribute("readonly", "");
  await expect(adminLogRow).toContainText(
    "留空等同于群号 0：不另发处罚记录；处罚失败提醒仍发到当前群。"
  );
});

test("moderation sends one sparse change with CSRF and accepts an unchanged revision", async ({
  page
}) => {
  let markPatchRequested!: () => void;
  let releasePatch!: () => void;
  const patchRequested = new Promise<void>((resolve) => {
    markPatchRequested = resolve;
  });
  const patchResponse = new Promise<void>((resolve) => {
    releasePatch = resolve;
  });
  let requestBody: unknown;
  let csrfHeader: string | undefined;

  await openModeration(
    page,
    async (route) => fulfillJSON(route, settingsPayload()),
    async (route) => {
      const request = route.request();
      requestBody = request.postDataJSON();
      csrfHeader = request.headers()["x-csrf-token"];
      markPatchRequested();
      await patchResponse;
      await fulfillJSON(
        route,
        settingsPayload({
          revision: 7,
          antispam_enabled: { value: false, source: "chat override" }
        })
      );
    }
  );

  await page.getByRole("switch", { name: "拦截频道马甲" }).click();
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await patchRequested;
  expect(csrfHeader).toBe("moderation-csrf");
  expect(requestBody).toEqual({
    expected_revision: 7,
    changes: { antispam_enabled: false }
  });
  await expect(page.locator("[data-moderation-savebar]")).toHaveAttribute(
    "data-save-state",
    "submitting"
  );
  await expect(page.getByRole("button", { name: "正在保存…" })).toHaveAttribute(
    "aria-disabled",
    "true"
  );

  releasePatch();
  await expect(page.locator("[data-moderation-form]")).toHaveAttribute(
    "data-save-state",
    "saved"
  );
  await expect(page.getByRole("switch", { name: "拦截频道马甲" })).toHaveAttribute(
    "aria-checked",
    "false"
  );
  await expect(page.locator('[data-moderation-feedback="saved"]')).toContainText(
    "管理与处罚设置已保存。"
  );
});

test("moderation restores one override with an explicit null", async ({ page }) => {
  let requestBody: unknown;
  await openModeration(
    page,
    async (route) => {
      await fulfillJSON(
        route,
        settingsPayload({ warn_limit: { value: 5, source: "chat override" } })
      );
    },
    async (route) => {
      requestBody = route.request().postDataJSON();
      await fulfillJSON(
        route,
        settingsPayload({
          revision: 8,
          warn_limit: { value: 3, source: "factory default" }
        })
      );
    }
  );

  const warningRow = page.locator('[data-moderation-field="warnLimit"]');
  await warningRow.getByRole("button", { name: "恢复默认" }).click();
  await expect(warningRow.locator("[data-setting-pending]")).toContainText("待恢复默认");
  await page.getByRole("button", { name: "保存", exact: true }).click();

  await expect(page.locator("[data-moderation-form]")).toHaveAttribute(
    "data-save-state",
    "saved"
  );
  expect(requestBody).toEqual({ expected_revision: 7, changes: { warn_limit: null } });
  await expect(warningRow.getByLabel("警告上限")).toHaveValue("3");
  await expect(warningRow).toContainText("出厂默认");
  await expect(warningRow.getByRole("button", { name: "恢复默认" })).toHaveCount(0);
});

test("moderation identifies another administrator's revision conflict before reloading", async ({
  page
}) => {
  let requestBody: unknown;
  await openModeration(
    page,
    async (route, requestNumber) => {
      await fulfillJSON(
        route,
        settingsPayload({
          revision: requestNumber === 1 ? 7 : 8,
          admin_log_chat_id: {
            value: requestNumber === 1 ? -1007000000001 : -1007000000002,
            source: "chat override"
          }
        })
      );
    },
    async (route) => {
      requestBody = route.request().postDataJSON();
      await fulfillJSON(route, { error: { code: "settings_conflict" } }, 409);
    }
  );

  const adminLogInput = page.getByLabel("处罚记录群");
  await adminLogInput.fill("");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.locator('[data-moderation-feedback="conflict"]')).toContainText(
    "别人改过这些设置了。重新载入后再保存。"
  );
  await expect(page.getByText("无法保存管理与处罚设置。请重试。")).toHaveCount(0);
  expect(requestBody).toEqual({
    expected_revision: 7,
    changes: { admin_log_chat_id: 0 }
  });

  await page.getByRole("button", { name: "重新载入" }).click();
  await expect(page.locator("[data-moderation-page]")).toHaveAttribute(
    "data-moderation-state",
    "loaded"
  );
  await expect(adminLogInput).toHaveValue("-1007000000002");
  await expect(page.locator("[data-moderation-savebar]")).toHaveCount(0);
  await expect(page.locator('[data-moderation-feedback="conflict"]')).toHaveCount(0);
});
