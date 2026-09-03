import { expect, test, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

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

  await expect(page.locator("#verification-mode")).toHaveAttribute("data-value", "kernel");
  await expect(page.getByText("来源：配置文件")).toBeVisible();
  await selectAppOption(page.locator("#verification-mode"), "quiz");
  await page.getByRole("button", { name: "保存更改" }).click();
  await patchSettled;
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "loaded"
  );
  await expect(page.locator("#verification-mode")).toHaveAttribute("data-value", "quiz");
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

  await selectAppOption(page.locator("#verification-mode"), "quiz");
  await page.getByRole("button", { name: "保存更改" }).click();
  await latestSettingsSettled;
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "loaded"
  );
  await expect(page.locator("#verification-mode")).toHaveAttribute("data-value", "mixed");
  await expect(page.locator("[data-verification-feedback]")).toContainText(
    "其他管理员已经更改了设置"
  );
  expect(reads).toBe(2);
});

test("verification discards a previous group's delayed settings response", async ({ page }) => {
  let markSettingsRequested!: () => void;
  let releaseSettings!: () => void;
  const settingsRequested = new Promise<void>((resolve) => {
    markSettingsRequested = resolve;
  });
  const readResponse = new Promise<void>((resolve) => {
    releaseSettings = resolve;
  });
  let markSettingsResponseSettled!: () => void;
  const settingsResponseSettled = new Promise<void>((resolve) => {
    markSettingsResponseSettled = resolve;
  });

  await mockVerificationTransport(
    page,
    async (route) => {
      markSettingsRequested();
      await readResponse;
      await fulfillJSON(route, settingsResponse());
      markSettingsResponseSettled();
    },
    async () => {
      throw new Error("A settings write is not part of a stale settings read");
    }
  );

  await page.goto(`/verification?group=${selectedGroupID}`, { waitUntil: "domcontentloaded" });
  await settingsRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), "all");
  await expect(page).toHaveURL(/\/verification$/);
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "group-required"
  );

  releaseSettings();
  await settingsResponseSettled;
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "group-required"
  );
  await expect(page.locator("[data-verification-form]")).toHaveCount(0);
});

test("verification ignores a previous group's delayed settings save", async ({ page }) => {
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

  await openLiveVerification(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async (route) => {
      markPatchRequested();
      await patchResponse;
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 8,
          verify_mode: sourced("quiz", "chat override")
        })
      );
      markPatchResponseSettled();
    }
  );

  await selectAppOption(page.locator("#verification-mode"), "quiz");
  await page.getByRole("button", { name: "保存更改" }).click();
  await patchRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), "all");
  await expect(page).toHaveURL(/\/verification$/);
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "group-required"
  );

  releasePatch();
  await patchResponseSettled;
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "group-required"
  );
  await expect(page.locator("[data-verification-form]")).toHaveCount(0);
});

test("verification rejects durations the bot cannot honour and saves valid boundaries", async ({
  page
}) => {
  const maximumDurationSeconds = 366 * 24 * 60 * 60;
  let patchRequests = 0;
  let allowPatch = false;

  await openLiveVerification(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async (route) => {
      patchRequests += 1;
      if (!allowPatch) {
        throw new Error("A duration the bot cannot honour reached the settings write");
      }
      expect(JSON.parse(route.request().postData() ?? "{}")).toEqual({
        expected_revision: 7,
        changes: {
          timeout_seconds: 30,
          verify_retry_seconds: maximumDurationSeconds,
          mute_seconds: 30
        }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 8,
          timeout_seconds: sourced(30, "chat override"),
          verify_retry_seconds: sourced(maximumDurationSeconds, "chat override"),
          mute_seconds: sourced(30, "chat override")
        })
      );
    }
  );

  const invalidDurations = [
    ["#verification-timeout-seconds", "29", "30"],
    ["#verification-timeout-seconds", "1801", "30"],
    ["#verification-retry-seconds", String(maximumDurationSeconds + 1), "180"],
    ["#verification-ban-seconds", "1", "0"],
    ["#verification-ban-seconds", "29", "0"],
    ["#verification-ban-seconds", String(maximumDurationSeconds + 1), "0"],
    ["#verification-mute-seconds", "29", "3600"],
    ["#verification-mute-seconds", String(maximumDurationSeconds + 1), "3600"]
  ] as const;

  for (const [selector, invalid, valid] of invalidDurations) {
    const field = page.locator(selector);
    await field.fill(invalid);
    await page.getByRole("button", { name: "保存更改" }).click();
    await expect(field).toHaveAttribute("aria-invalid", "true");
    await expect(page.locator(`${selector}-error`)).toBeVisible();
    expect(patchRequests).toBe(0);
    await field.fill(valid);
  }

  allowPatch = true;
  await page.locator("#verification-timeout-seconds").fill("30");
  await page.locator("#verification-retry-seconds").fill(String(maximumDurationSeconds));
  await page.locator("#verification-ban-seconds").fill("0");
  await page.locator("#verification-mute-seconds").fill("30");
  await page.getByRole("button", { name: "保存更改" }).click();

  await expect(page.locator("[data-verification-feedback]")).toContainText("已保存验证设置");
  expect(patchRequests).toBe(1);
});

test("verification retains a draft through transient access failures and removes it after access is revoked", async ({
  page
}) => {
  let writes = 0;

  await openLiveVerification(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async (route) => {
      writes += 1;
      if (writes === 1) {
        await fulfillJSON(route, { error: { code: "zz_never_a_code" } }, 503);
        return;
      }
      if (writes === 2) {
        await fulfillJSON(route, { error: { code: "chat_access_unavailable" } }, 503);
        return;
      }
      if (writes === 3) {
        await fulfillJSON(
          route,
          settingsResponse({
            revision: 8,
            verify_mode: sourced("quiz", "chat override")
          })
        );
        return;
      }
      await fulfillJSON(route, { error: { code: "chat_access_denied" } }, 403);
    }
  );

  await selectAppOption(page.locator("#verification-mode"), "quiz");
  for (const expectedWrites of [1, 2]) {
    await page.getByRole("button", { name: "保存更改" }).click();
    await expect.poll(() => writes).toBe(expectedWrites);
    await expect(page.locator("[data-verification-page]")).toHaveAttribute(
      "data-verification-state",
      "loaded"
    );
    await expect(page.locator("#verification-mode")).toHaveAttribute("data-value", "quiz");
  }

  await page.getByRole("button", { name: "保存更改" }).click();
  await expect.poll(() => writes).toBe(3);
  await expect(page.locator("[data-verification-feedback]")).toContainText("已保存验证设置");

  await selectAppOption(page.locator("#verification-mode"), "kernel");
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect.poll(() => writes).toBe(4);
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "unavailable"
  );
  await expect(page.locator("[data-verification-form]")).toHaveCount(0);
});
