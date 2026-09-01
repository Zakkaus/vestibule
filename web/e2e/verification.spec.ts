import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1001163306055";
const actorID = "741928306";

type SettingSource = "factory default" | "user file" | "chat override";

type SourcedSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

type SettingsResponse = Readonly<{
  revision: number;
  delivery_mode: SourcedSetting<"group" | "dm" | "both">;
  verify_mode: SourcedSetting<"kernel" | "quiz" | "mixed">;
  timeout_seconds: SourcedSetting<number>;
  verify_max_fails: SourcedSetting<number>;
  verify_retry_seconds: SourcedSetting<number>;
  ban_seconds: SourcedSetting<number>;
  mute_seconds: SourcedSetting<number>;
  verify_invited: SourcedSetting<boolean>;
}>;

type SettingsReadHandler = (route: Route) => Promise<void>;
type SettingsPatchHandler = (route: Route) => Promise<void>;

type SettingsOverrides = Partial<SettingsResponse>;

function sourced<T>(value: T, source: SettingSource = "factory default"): SourcedSetting<T> {
  return { value, source };
}

function settingsResponse(overrides: SettingsOverrides = {}): SettingsResponse {
  return {
    revision: 7,
    delivery_mode: sourced("both"),
    verify_mode: sourced("kernel"),
    timeout_seconds: sourced(240, "user file"),
    verify_max_fails: sourced(3),
    verify_retry_seconds: sourced(180),
    ban_seconds: sourced(0),
    mute_seconds: sourced(3600),
    verify_invited: sourced(true),
    ...overrides
  };
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockVerificationTransport(
  page: Page,
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2026-09-01T02:00:00Z",
        csrf_token: "verification-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID }] });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "GET") {
      await readSettings(route);
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "PATCH") {
      await patchSettings(route);
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openLiveVerification(
  page: Page,
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler
): Promise<void> {
  await mockVerificationTransport(page, readSettings, patchSettings);
  await page.goto(`/verification?group=${selectedGroupID}`);
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "loaded"
  );
}

test("verification saves only the edited field through the shared CSRF transport", async ({ page }) => {
  let markPatchSettled!: () => void;
  const patchSettled = new Promise<void>((resolve) => {
    markPatchSettled = resolve;
  });

  await openLiveVerification(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async (route) => {
      const request = route.request();
      expect(request.headers()["x-csrf-token"]).toBe("verification-csrf");
      expect(JSON.parse(request.postData() ?? "{}")).toEqual({
        expected_revision: 7,
        changes: { verify_mode: "quiz" }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 8,
          verify_mode: sourced("quiz", "chat override")
        })
      );
      markPatchSettled();
    }
  );

  await expect(page.locator("#verification-mode")).toHaveValue("kernel");
  await expect(page.getByText("来源：配置文件")).toBeVisible();
  await page.locator("#verification-mode").selectOption("quiz");
  await page.getByRole("button", { name: "保存更改" }).click();
  await patchSettled;
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "loaded"
  );
  await expect(page.locator("#verification-mode")).toHaveValue("quiz");
  await expect(page.getByText("来源：此群覆盖")).toBeVisible();
  await expect(page.locator("[data-verification-feedback]")).toContainText("已保存验证设置");
});

test("verification restores only the selected chat override with null", async ({ page }) => {
  let markPatchSettled!: () => void;
  const patchSettled = new Promise<void>((resolve) => {
    markPatchSettled = resolve;
  });

  await openLiveVerification(
    page,
    async (route) => {
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 5,
          ban_seconds: sourced(3600, "chat override")
        })
      );
    },
    async (route) => {
      expect(JSON.parse(route.request().postData() ?? "{}")).toEqual({
        expected_revision: 5,
        changes: { ban_seconds: null }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 6,
          ban_seconds: sourced(0)
        })
      );
      markPatchSettled();
    }
  );

  const banSetting = page.locator("[data-verification-setting=ban_seconds]");
  await expect(banSetting).toContainText("来源：此群覆盖");
  await banSetting.getByRole("button", { name: "恢复继承值" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await patchSettled;
  await expect(page.locator("#verification-ban-seconds")).toHaveValue("0");
  await expect(banSetting).toContainText("来源：出厂默认");
  await expect(page.locator("[data-verification-feedback]")).toContainText("已保存验证设置");
});

test("verification conflict loads the newer revision and says another administrator changed it", async ({
  page
}) => {
  let reads = 0;
  let markLatestSettingsSettled!: () => void;
  const latestSettingsSettled = new Promise<void>((resolve) => {
    markLatestSettingsSettled = resolve;
  });

  await openLiveVerification(
    page,
    async (route) => {
      reads += 1;
      if (reads === 1) {
        await fulfillJSON(route, settingsResponse({ revision: 12 }));
        return;
      }
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 13,
          verify_mode: sourced("mixed", "chat override")
        })
      );
      markLatestSettingsSettled();
    },
    async (route) => {
      expect(JSON.parse(route.request().postData() ?? "{}")).toEqual({
        expected_revision: 12,
        changes: { verify_mode: "quiz" }
      });
      await fulfillJSON(route, { error: { code: "settings_conflict" } }, 409);
    }
  );

  await page.locator("#verification-mode").selectOption("quiz");
  await page.getByRole("button", { name: "保存更改" }).click();
  await latestSettingsSettled;
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "loaded"
  );
  await expect(page.locator("#verification-mode")).toHaveValue("mixed");
  await expect(page.locator("[data-verification-feedback]")).toContainText(
    "其他管理员已经更改了设置"
  );
  expect(reads).toBe(2);
});

test("verification rejects an invalid timeout before it starts a PATCH request", async ({ page }) => {
  let patchRequests = 0;

  await openLiveVerification(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async () => {
      patchRequests += 1;
      throw new Error("Validation must prevent PATCH requests with an invalid timeout");
    }
  );

  await page.locator("#verification-timeout-seconds").fill("29");
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(page.locator("#verification-timeout-seconds")).toHaveAttribute(
    "aria-invalid",
    "true"
  );
  await expect(page.locator("#verification-timeout-seconds-error")).toContainText("30 至 1,800");
  expect(patchRequests).toBe(0);
});
