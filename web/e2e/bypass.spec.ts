import { expect, test, type Page, type Route } from "@playwright/test";
import { selectAppOption } from "./app-select";

const selectedGroupID = "-1001163306055";
const otherGroupID = "-1009000000003";
const actorID = "741928306";

type SettingSource = "factory default" | "user file" | "chat override";

type SourcedSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

type SettingsResponse = Readonly<{
  revision: number;
  trusted_member_group_ids: SourcedSetting<readonly number[]>;
  required_channel_id: SourcedSetting<number>;
  required_channel_fail_open: SourcedSetting<boolean>;
  channel_display: SourcedSetting<string>;
  channel_invite_url: SourcedSetting<string>;
  channel_whitelist: SourcedSetting<readonly number[]>;
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
    trusted_member_group_ids: sourced([-1007000000001]),
    required_channel_id: sourced(-1008000000001),
    required_channel_fail_open: sourced(true),
    channel_display: sourced("@required"),
    channel_invite_url: sourced(""),
    channel_whitelist: sourced([-1009000000001]),
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

async function mockBypassTransport(
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
        csrf_token: "bypass-csrf"
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

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openLiveBypass(
  page: Page,
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler
): Promise<void> {
  await mockBypassTransport(page, readSettings, patchSettings);
  await page.goto(`/bypass?group=${selectedGroupID}`);
  await expect(page.locator("[data-bypass-page]")).toHaveAttribute("data-bypass-state", "loaded");
}

test("bypass saves only an edited trusted-group list through the shared CSRF transport", async ({
  page
}) => {
  let markPatchSettled!: () => void;
  const patchSettled = new Promise<void>((resolve) => {
    markPatchSettled = resolve;
  });

  await openLiveBypass(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async (route) => {
      const request = route.request();
      expect(request.headers()["x-csrf-token"]).toBe("bypass-csrf");
      expect(JSON.parse(request.postData() ?? "{}")).toEqual({
        expected_revision: 7,
        changes: { trusted_member_group_ids: [-1007000000001, -1007000000002] }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 8,
          trusted_member_group_ids: sourced(
            [-1007000000001, -1007000000002],
            "chat override"
          )
        })
      );
      markPatchSettled();
    }
  );

  await page.locator("#bypass-trusted-member-group-ids").fill("-1007000000001\n-1007000000002");
  await page.getByRole("button", { name: "保存" }).click();
  await patchSettled;
  await expect(page.locator("[data-bypass-page]")).toHaveAttribute("data-bypass-state", "loaded");
  await expect(page.locator("#bypass-trusted-member-group-ids")).toHaveValue(
    "-1007000000001\n-1007000000002"
  );
  await expect(page.locator("[data-bypass-feedback]")).toContainText("免验证来源设置已保存");
});

test("bypass restores only the selected channel invite override with null", async ({ page }) => {
  let markPatchSettled!: () => void;
  const patchSettled = new Promise<void>((resolve) => {
    markPatchSettled = resolve;
  });

  await openLiveBypass(
    page,
    async (route) => {
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 5,
          channel_invite_url: sourced("https://t.me/+private", "chat override")
        })
      );
    },
    async (route) => {
      expect(JSON.parse(route.request().postData() ?? "{}")).toEqual({
        expected_revision: 5,
        changes: { channel_invite_url: null }
      });
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 6,
          channel_invite_url: sourced("")
        })
      );
      markPatchSettled();
    }
  );

  const inviteSetting = page.locator("[data-bypass-setting=channelInviteURL]");
  await expect(inviteSetting).toContainText("本群设定");
  await inviteSetting.getByRole("button", { name: "恢复默认" }).click();
  await page.getByRole("button", { name: "保存" }).click();
  await patchSettled;
  await expect(page.locator("#bypass-channel-invite-url")).toHaveValue("");
  await expect(inviteSetting).toContainText("出厂默认");
  await expect(page.locator("[data-bypass-feedback]")).toContainText("免验证来源设置已保存");
});

test("bypass conflict reloads the newest revision and tells the administrator", async ({ page }) => {
  let reads = 0;

  await openLiveBypass(
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
          required_channel_fail_open: sourced(false, "chat override")
        })
      );
    },
    async (route) => {
      expect(JSON.parse(route.request().postData() ?? "{}")).toEqual({
        expected_revision: 12,
        changes: { required_channel_fail_open: false }
      });
      await fulfillJSON(route, { error: { code: "settings_conflict" } }, 409);
    }
  );

  await page.locator("#bypass-required-channel-fail-open").click();
  await page.getByRole("button", { name: "保存" }).click();
  await expect.poll(() => reads).toBe(2);
  await expect(page.locator("[data-bypass-page]")).toHaveAttribute("data-bypass-state", "loaded");
  await expect(page.locator("#bypass-required-channel-fail-open")).toHaveAttribute(
    "aria-checked",
    "false"
  );
  await expect(page.locator("[data-bypass-feedback]")).toContainText("别人改过这些设置了");
});

test("bypass rejects an invalid ID list before it starts a PATCH request", async ({ page }) => {
  let patchRequests = 0;

  await openLiveBypass(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async () => {
      patchRequests += 1;
      throw new Error("Validation must prevent a PATCH request with an invalid ID list");
    }
  );

  await page.locator("#bypass-channel-whitelist").fill("-1009000000001\n0");
  await expect(page.getByRole("button", { name: "保存" })).toHaveAttribute(
    "aria-disabled",
    "true"
  );
  await expect(page.locator("#bypass-channel-whitelist")).toHaveAttribute("aria-invalid", "true");
  await expect(page.locator("#bypass-channel-whitelist-error")).toContainText("非 0 的整数");
  expect(patchRequests).toBe(0);
});

test("bypass rejects a required channel without a reachable name or invite", async ({ page }) => {
  let patchRequests = 0;

  await openLiveBypass(
    page,
    async (route) => {
      await fulfillJSON(
        route,
        settingsResponse({
          required_channel_id: sourced(0),
          channel_display: sourced(""),
          channel_invite_url: sourced("")
        })
      );
    },
    async () => {
      patchRequests += 1;
      throw new Error("Validation must prevent a PATCH request with an unreachable channel");
    }
  );

  await page.locator("#bypass-required-channel-id").fill("-1008000000001");
  await expect(page.getByRole("button", { name: "保存" })).toHaveAttribute(
    "aria-disabled",
    "true"
  );
  await expect(page.locator("#bypass-required-channel-id")).toHaveAttribute("aria-invalid", "true");
  await expect(page.locator("#bypass-required-channel-id-error")).toContainText("显示名或频道邀请链接");
  expect(patchRequests).toBe(0);
});

test("bypass explains the risk of each required-channel failure direction", async ({ page }) => {
  await openLiveBypass(
    page,
    async (route) => {
      await fulfillJSON(route, settingsResponse());
    },
    async () => {
      throw new Error("The failure-direction explanation must not send a PATCH request");
    }
  );

  const setting = page.locator("[data-bypass-setting=requiredChannelFailOpen]");
  await expect(setting).toContainText("未关注频道的人也可能进入");
  await page.locator("#bypass-required-channel-fail-open").click();
  await expect(setting).toContainText("正常申请人也可能被误拒");
});

test("bypass discards a previous group's delayed settings response", async ({ page }) => {
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

  await mockBypassTransport(
    page,
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === `/api/chats/${selectedGroupID}/settings`) {
        markSettingsRequested();
        await firstSettingsResponse;
        await fulfillJSON(route, settingsResponse({ channel_display: sourced("@group-a") }));
        markSettingsResponseSettled();
        return;
      }
      await fulfillJSON(
        route,
        settingsResponse({ revision: 8, channel_display: sourced("@group-b", "chat override") })
      );
    },
    async () => {
      throw new Error("A delayed read must not write bypass settings");
    }
  );

  await page.goto(`/bypass?group=${selectedGroupID}`, { waitUntil: "domcontentloaded" });
  await settingsRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), otherGroupID);
  await expect(page).toHaveURL(new RegExp(`/bypass\\?group=${otherGroupID}$`));
  await expect(page.locator("#bypass-channel-display")).toHaveValue("@group-b");

  releaseSettings();
  await settingsResponseSettled;
  await expect(page.locator("#bypass-channel-display")).toHaveValue("@group-b");
});

test("bypass ignores a previous group's delayed settings save", async ({ page }) => {
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

  await openLiveBypass(
    page,
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      await fulfillJSON(
        route,
        path === `/api/chats/${otherGroupID}/settings`
          ? settingsResponse({
              revision: 8,
              channel_display: sourced("@group-b", "chat override")
            })
          : settingsResponse()
      );
    },
    async (route) => {
      markPatchRequested();
      await patchResponse;
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 8,
          channel_display: sourced("@group-a", "chat override")
        })
      );
      markPatchResponseSettled();
    }
  );

  await page.locator("#bypass-channel-display").fill("@group-a");
  await page.getByRole("button", { name: "保存" }).click();
  await patchRequested;
  await selectAppOption(page.getByRole("button", { name: "当前群" }), otherGroupID);
  await expect(page).toHaveURL(new RegExp(`/bypass\\?group=${otherGroupID}$`));
  await expect(page.locator("#bypass-channel-display")).toHaveValue("@group-b");

  releasePatch();
  await patchResponseSettled;
  await expect(page.locator("#bypass-channel-display")).toHaveValue("@group-b");
});
