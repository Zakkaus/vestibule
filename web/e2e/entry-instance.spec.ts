import { expect, test } from "@playwright/test";

async function serveInstance(page: import("@playwright/test").Page, botUsername: string | null) {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/instance") {
      if (botUsername === null) {
        await route.fulfill({ status: 500, body: "" });
        return;
      }
      await route.fulfill({
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ bot_username: botUsername })
      });
      return;
    }
    await route.fulfill({
      status: 401,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ error: { code: "authentication_expired" } })
    });
  });
}

// The handle used to be a constant in the bundle, so every instance told its
// operator to open the previous generation's bot.
test("the entry screen names the bot the instance reports", async ({ page }) => {
  await serveInstance(page, "example_console_bot");
  await page.goto("/");

  const entry = page.locator("[data-entry-page]");
  await expect(entry).toContainText("@example_console_bot");
  await expect(entry).not.toContainText("gentoo_zh_verify_bot");
});

// An unclaimed instance has no bot, so it cannot send anyone to Telegram to
// open one; the way in is the link the install script printed.
test("an unclaimed instance says so and names no bot", async ({ page }) => {
  await serveInstance(page, "");
  await page.goto("/");

  const entry = page.locator("[data-entry-page]");
  await expect(entry).toHaveAttribute("data-entry-state", "unclaimed");
  expect(await entry.innerText()).not.toContain("@");
});

// A console that cannot be reached is not a console nobody has claimed. Reading
// one as the other sends somebody hunting for an install link that was consumed
// weeks ago.
test("an unreachable instance is not reported as unclaimed", async ({ page }) => {
  await serveInstance(page, null);
  await page.goto("/");

  const entry = page.locator("[data-entry-page]");
  await expect(entry).toBeVisible();
  await expect(entry).not.toHaveAttribute("data-entry-state", "unclaimed");
});
