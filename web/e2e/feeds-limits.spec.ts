import { expect, test } from "@playwright/test";

const chatID = "-1009000000202";
const setting = <T,>(value: T) => ({ value, source: "factory default" });

for (const locale of ["en", "zh-CN", "zh-TW", "ja", "ru"]) {
  test(`feeds preserves the draft and shows deployer cap details in ${locale}`, async ({ page }) => {
    await page.addInitScript((language) => {
      localStorage.setItem("verify-console-locale", language);
    }, locale);
    await page.route("**/api/**", async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      let body: unknown;
      let status = 200;
      if (path === "/api/session") {
        body = {
          subject: { telegram_id: "9000000201", role: "manager" },
          expires_at: "2026-10-04T12:00:00Z", is_owner: false, csrf_token: "feed-limit-csrf"
        };
      } else if (path === "/api/chats") {
        body = { chats: [{ id: chatID, title: "Limit test group", owner: null, administrators: [], administrators_status: "unavailable" }] };
      } else if (path === "/api/process/settings") {
        status = 403;
        body = { error: { code: "process_access_denied" } };
      } else if (path === `/api/chats/${chatID}/feeds` && request.method() === "GET") {
        body = {
          revision: 4,
          feed: {
            lang: setting(""), interval_seconds: setting(300), bugs: setting(false),
            bugzilla_base: setting(""), news: setting(false), bug_product: setting(""),
            bug_component: setting(""), silent_bugs: setting(false)
          },
          github_repos: setting([])
        };
      } else if (path === `/api/chats/${chatID}/feeds` && request.method() === "PUT") {
        status = 400;
        body = {
          error: { code: "settings_limit_exceeded" },
          violations: [{ chat_id: chatID, field: "timeout_seconds", value: 240, limit: 30 }]
        };
      } else {
        throw new Error(`Unexpected API request: ${request.method()} ${path}`);
      }
      await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
    });

    await page.goto(`/feeds?group=${chatID}`);
    const interval = page.locator("#feeds-interval-seconds");
    await interval.fill("600");
    await page.locator("[data-feeds-savebar]").getByRole("button").last().click();
    const feedback = page.locator("[data-feeds-feedback]");
    await expect(feedback).toHaveAttribute("data-tone", "error");
    await expect(feedback).toHaveAttribute("role", "alert");
    const violation = feedback.locator("[data-settings-limit-violations] li");
    await expect(violation).toContainText(chatID);
    await expect(violation).toContainText("240");
    await expect(violation).toContainText("30");
    await expect(violation).not.toContainText("timeout_seconds");
    await expect(interval).toHaveValue("600");
    await expect(page.locator("[data-feeds-page]")).toHaveAttribute("data-feeds-state", "loaded");
    await expect(page.locator("html")).toHaveAttribute("lang", locale);
  });
}
