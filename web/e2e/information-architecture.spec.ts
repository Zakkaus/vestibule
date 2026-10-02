import { expect, test, type Page } from "@playwright/test";
import { unmeasuredDiagnostics } from "./diagnostics-fixtures";

const groupID = "-1009000000811";
const knownID = -1009000000812;
const sourced = <T>(value: T, source = "user file") => ({ value, source });

async function mockIA(page: Page, role = "manager", noGroups = false) {
  let revision = 4;
  let known = [knownID, -1009000000813];
  let conflict = false;
  const requests: string[] = [];
  const changes: Record<string, unknown>[] = [];
  await page.route("**/api/**", async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    requests.push(`${request.method()} ${path}`);
    const reply = (body: unknown, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
    if (path === "/api/session") return reply({ subject: { telegram_id: "9000000811", role }, expires_at: "2099-01-01T00:00:00Z", is_owner: false, csrf_token: "ia-csrf" });
    if (path === "/api/instance") return reply({ bot_username: "example_bot" });
    if (path === "/api/chats") return reply({ chats: noGroups ? [] : [{ id: groupID, title: "Example group", owner: null, administrators: [], administrators_status: "unavailable" }] });
    if (path === "/api/status") return reply(unmeasuredDiagnostics);
    if (path === "/api/status/daily") return reply({ enabled: true, time: "09:00", timezone: "UTC" });
    if (path === "/api/process/settings") return reply({ news_url: sourced("https://example.invalid/news.xml"), overlays: sourced([{ name: "Example overlay", repo: "example/repository", branch: "main" }]), private_query_per_min: sourced(8) });
    if (path.endsWith("/feeds")) {
      if (request.method() === "PUT") return reply({ error: { code: "settings_limit_exceeded" }, violations: [{ chat_id: groupID, field: "timeout_seconds", value: 300, limit: 120 }] }, 400);
      const feed = { lang: "en", interval_seconds: 600, bugs: false, bugzilla_base: "", news: false, bug_product: "", bug_component: "", silent_bugs: false };
      return reply({ revision, feed: Object.fromEntries(Object.entries(feed).map(([key, value]) => [key, sourced(value)])), github_repos: sourced([]) });
    }
    if (path.endsWith("/settings")) {
      if (request.method() === "PATCH") {
        const body = request.postDataJSON();
        expect(request.headers()["x-csrf-token"]).toBe("ia-csrf");
        changes.push(body.changes);
        if (conflict || body.expected_revision !== revision) return reply({ error: { code: "settings_conflict" } }, 409);
        if (body.changes.known_chat_ids !== undefined) { known = body.changes.known_chat_ids ?? []; revision++; }
        else return reply({ error: { code: "settings_limit_exceeded" }, violations: [{ chat_id: groupID, field: "known_chat_ids", value: 2, limit: 1 }] }, 400);
      }
      return reply({ revision, known_chat_ids: sourced(known, revision > 4 ? "chat override" : "user file"), enabled: sourced(true), antispam_enabled: sourced(true), gentoo_lookups_enabled: sourced(true), linux_lookups_enabled: sourced(true), warn_limit: sourced(5), admin_log_chat_id: sourced(0), trusted_member_group_ids: sourced([]), required_channel_id: sourced(0), required_channel_fail_open: sourced(false), channel_display: sourced(""), channel_invite_url: sourced(""), channel_whitelist: sourced([]) });
    }
    throw new Error(`Unexpected IA request: ${request.method()} ${path}`);
  });
  return { requests, changes, conflict: () => { conflict = true; revision++; } };
}

for (const width of [390, 1280]) test(`manager repairs known chats from a cap notice at ${width}`, async ({ page }) => {
  await page.setViewportSize({ width, height: 844 });
  await page.addInitScript(() => localStorage.setItem("verify-console.locale", "en"));
  const harness = await mockIA(page);
  await page.goto(`/capabilities?group=${groupID}`);
  const control = page.locator('[data-capability-card="antispam"]').getByRole("switch");
  await control.focus();
  await page.keyboard.press("Space");
  await expect(control).toHaveAttribute("aria-checked", "false");
  await page.locator("[data-capabilities-savebar] button[type=submit]").click();
  const link = page.locator("[data-settings-limit-violations] a");
  await expect(page.locator("[data-settings-limit-violations]")).toContainText("Example group");
  await expect(page.locator("[data-settings-limit-violations]")).not.toContainText(groupID);
  await expect(link).toHaveAttribute("href", `/groups?group=${groupID}&edit=known-chats`);
  await link.click();
  const input = page.locator("[data-known-chats-editor] textarea");
  await expect(input).toBeEditable();
  await input.fill("0");
  await expect(input).toHaveAttribute("aria-invalid", "true");
  await expect(page.locator("[data-known-chats-editor] button[type=submit]")).toBeDisabled();
  await input.fill(String(knownID));
  await page.locator("[data-known-chats-editor] button[type=submit]").click();
  await expect(page.locator('[data-known-chats-editor] [data-setting-source]')).toHaveAttribute("data-setting-source", "chat override");
  await page.reload();
  await expect(input).toHaveValue(String(knownID));
  expect(harness.changes).toEqual([{ antispam_enabled: false }, { known_chat_ids: [knownID] }]);
  expect(harness.requests).not.toContain("GET /api/process/settings");
  await expect(page.locator('[data-navigation-item="/diagnostics"]')).toHaveCount(0);
});

test("known-chat conflict retains the draft and its baseline revision", async ({ page }) => {
  const harness = await mockIA(page);
  await page.goto(`/groups?group=${groupID}&edit=known-chats`);
  const input = page.locator("[data-known-chats-editor] textarea");
  await expect(input).toHaveValue(`${knownID}\n-1009000000813`);
  await input.fill(String(knownID));
  harness.conflict();
  await page.locator("[data-known-chats-editor] button[type=submit]").click();
  await expect(page.locator("[data-known-chats-editor] [role=alert]")).toBeVisible();
  await expect(input).toHaveValue(String(knownID));
});

for (const reloadFails of [false, true]) test(`known-chat reload locks edits and settles after ${reloadFails ? "failure" : "success"}`, async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  const harness = await mockIA(page);
  await page.goto(`/groups?group=${groupID}&edit=known-chats`);
  const editor = page.locator("[data-known-chats-editor]");
  const input = editor.locator("textarea");
  const save = editor.getByRole("button", { name: "Save changes", exact: true });
  await input.fill(String(knownID));
  await save.click();
  await expect(editor.locator("[data-setting-source]")).toHaveAttribute("data-setting-source", "chat override");
  const draft = "-1009000000814";
  await input.fill(draft);
  harness.conflict();
  await save.click();
  await expect(editor.getByRole("alert")).toBeVisible();
  let release!: () => void;
  const pending = new Promise<void>(resolve => { release = resolve; });
  await page.route(`**/api/chats/${groupID}/settings`, async route => {
    if (route.request().method() !== "GET") return route.fallback();
    await pending;
    if (reloadFails) return route.fulfill({ status: 503, json: { error: { code: "settings_unavailable" } } });
    return route.fulfill({ json: { revision: 6, known_chat_ids: sourced([knownID], "chat override") } });
  });
  await editor.getByRole("button", { name: "Discard draft and reload" }).click();
  await expect(editor).toHaveAttribute("aria-busy", "true");
  await expect(input).not.toBeEditable();
  await expect(save).toBeDisabled();
  await expect(editor.getByRole("button", { name: "Discard", exact: true })).toBeDisabled();
  await expect(editor.getByRole("button", { name: "Restore inherited value" })).toBeDisabled();
  await input.focus();
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.type("-1009000000815");
  await expect(input).toHaveValue(draft);
  release();
  await expect(editor).toHaveAttribute("aria-busy", "false");
  await expect(input).toBeEditable();
  await expect(input).toHaveValue(reloadFails ? draft : String(knownID));
  if (reloadFails) await expect(editor.getByRole("alert")).toBeVisible();
  await page.route(`**/api/chats/${groupID}/settings`, async route => {
    if (route.request().method() !== "PATCH") return route.fallback();
    expect(route.request().postDataJSON()).toEqual({
      expected_revision: reloadFails ? 5 : 6, changes: { known_chat_ids: [Number(draft)] }
    });
    return route.fulfill({ json: { revision: 7, known_chat_ids: sourced([Number(draft)], "chat override") } });
  });
  await input.fill(draft);
  await save.click();
  await expect(editor.getByRole("status")).toHaveText("Capability settings were saved.");
  await expect(input).toHaveValue(draft);
});

for (const failure of [
  { code: "authentication_expired", status: 401, message: "The session has expired. Open the console again from Telegram." },
  { code: "authentication_invalid", status: 401, message: "This console session cannot be verified. Open the console again from Telegram." },
  { code: "process_settings_unavailable", status: 503, message: "The process settings service is unavailable. Try again." }
]) test(`process settings explains ${failure.code}`, async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await mockIA(page, "operator", true);
  await page.route("**/api/process/settings", route => route.fulfill({
    status: failure.status, json: { error: { code: failure.code } }
  }));
  await page.goto("/diagnostics");
  await expect(page.locator("[data-process-settings]").getByRole("alert")).toContainText(failure.message);
  await expect(page.locator("[data-process-private-query-rate]")).toHaveCount(0);
});

test("operator reads process resources without manageable groups", async ({ page }) => {
  await mockIA(page, "operator", true);
  await page.goto("/diagnostics");
  await expect(page.locator("[data-process-private-query-rate]")).toHaveText("8");
  await expect(page.locator("[data-news-url-value]")).toContainText("https://example.invalid/news.xml");
  await expect(page.locator("[data-overlay-item]")).toContainText("example/repository");
  await expect(page.locator("[data-process-settings] input, [data-process-settings] textarea")).toHaveCount(0);
});

test("file-backed moderation and bypass controls accept keyboard edits", async ({ page }) => {
  await mockIA(page);
  await page.goto(`/moderation?group=${groupID}`);
  const warning = page.locator("#moderation-warnLimit-input");
  await expect(warning).toBeEditable();
  await warning.fill("7");
  await expect(warning).toHaveValue("7");
  await page.goto(`/bypass?group=${groupID}`);
  const list = page.locator("#bypass-channel-whitelist");
  await expect(list).toBeEditable();
  await list.fill(String(knownID));
  await expect(list).toHaveValue(String(knownID));
});

test("feed cap notices name the group and link to the violated field's editor", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console.locale", "zh-CN"));
  await mockIA(page);
  await page.goto(`/feeds?group=${groupID}`);
  await expect(page.locator("[data-feeds-form]")).toBeVisible();
  await page.locator("#feeds-interval-seconds").fill("900");
  await page.locator("[data-feeds-savebar]").getByRole("button").last().click();
  const notice = page.locator("[data-settings-limit-violations]");
  await expect(notice).toContainText("Example group");
  await expect(notice).not.toContainText(groupID);
  await expect(page.locator("[data-feeds-feedback]")).toContainText("部署者");
  await expect(notice.getByRole("link")).toHaveAttribute("href", `/verification?group=${groupID}`);
  await expect(page.locator("#feeds-interval-seconds")).toHaveValue("900");
});
