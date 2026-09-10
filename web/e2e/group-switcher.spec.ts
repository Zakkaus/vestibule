import { expect, test, type Page, type Route } from "@playwright/test";

const namedGroupId = "-1004237282609";
const unnamedGroupId = "-1001834029912";
const blankGroupId = "-1006725039401";

async function fulfillJSON(route: Route, body: unknown): Promise<void> {
  await route.fulfill({
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockGroupSwitcher(
  page: Page,
  chats: readonly Readonly<{ id: string; title?: string }>[]
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: "741928306", role: "manager" },
        expires_at: "2030-09-04T12:00:00Z",
        csrf_token: "group-switcher-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats });
      return;
    }
    if (path === "/api/instance" && request.method() === "GET") {
      await fulfillJSON(route, { bot_username: "example_bot" });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

test("a chat with a missing title falls back to its group ID", async ({ page }) => {
  const title = "维护者讨论群";
  await mockGroupSwitcher(page, [
    { id: namedGroupId, title },
    { id: unnamedGroupId },
    { id: blankGroupId, title: "   " }
  ]);
  await page.goto(`/groups?group=${namedGroupId}`);

  const trigger = page.getByRole("button", { name: "当前群" });
  await expect(trigger).toHaveText(title);
  await expect(trigger).not.toContainText(/-100\d+/);
  await expect(page.locator("[data-group-row][data-selected]")).not.toContainText(/-100\d+/);
  await expect(page.locator("[data-group-row][aria-current]")).toHaveCount(1);
  await expect(page.locator("[data-group-row]").first()).toHaveAttribute("aria-current", "true");
  await expect(page.locator("[data-group-row]").nth(1)).not.toHaveAttribute("aria-current", "true");
  await expect(page.locator("[data-group-row]").first().getByRole("link")).toHaveAttribute(
    "href",
    `/queue?group=${namedGroupId}`
  );
  await trigger.click();
  await expect(page.getByRole("option", { name: title, exact: true })).toBeVisible();

  await page.getByRole("option", { name: unnamedGroupId, exact: true }).click();
  await expect(trigger).toHaveText(unnamedGroupId);
  await expect(page).toHaveURL(new RegExp(`/groups\\?group=${unnamedGroupId}$`));
  await expect(page.locator("[data-group-row][data-selected]").getByRole("heading")).toHaveText(unnamedGroupId);
  await expect(page.locator("[data-group-row][aria-current]")).toHaveCount(1);
  await expect(page.locator("[data-group-row]").nth(1)).toHaveAttribute("aria-current", "true");
  await trigger.click();
  await page.getByRole("option", { name: blankGroupId, exact: true }).click();
  await expect(trigger).toHaveText(blankGroupId);
  await expect(page.locator("[data-group-row][data-selected]").getByRole("heading")).toHaveText(blankGroupId);
  await expect(page.locator("[data-group-row][aria-current]")).toHaveCount(1);
  await expect(page.locator("[data-group-row]").nth(2)).toHaveAttribute("aria-current", "true");
  await trigger.click();
  await page.getByRole("option", { name: "全部可管理的群", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/groups$`));
  await expect(page.locator("[data-group-row][aria-current]")).toHaveCount(0);
  await expect(page.locator("[data-group-row][data-selected]")).toHaveCount(0);
});

test("fixture group selection exposes one current item and preserves queue routing", async ({ page }) => {
  const fixtureGroupId = "-1001163306055";
  const secondFixtureGroupId = "-1001834029912";
  await page.goto(`/groups?group=${fixtureGroupId}`);

  const rows = page.locator("[data-group-row]");
  await expect(rows).toHaveCount(3);
  await expect(page.locator("[data-group-row][aria-current]")).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute("aria-current", "true");
  await expect(rows.nth(1)).not.toHaveAttribute("aria-current", "true");
  await expect(rows.first().getByRole("link")).toHaveAttribute(
    "href",
    `/queue?group=${fixtureGroupId}`
  );

  const trigger = page.getByRole("button", { name: "当前群" });
  await trigger.click();
  await page.getByRole("option", { name: "Arch Linux 中文社区", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/groups\\?group=${secondFixtureGroupId}$`));
  await expect(page.locator("[data-group-row][aria-current]")).toHaveCount(1);
  await expect(rows.nth(1)).toHaveAttribute("aria-current", "true");
  await expect(rows.first()).not.toHaveAttribute("aria-current", "true");

  await trigger.click();
  await page.getByRole("option", { name: "全部可管理的群", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/groups$`));
  await expect(page.locator("[data-group-row][aria-current]")).toHaveCount(0);
  await expect(page.locator("[data-group-row][data-selected]")).toHaveCount(0);
});

test("titled group pages and switcher selections never expose transport IDs", async ({ page }) => {
  const title = "Maintainers Workspace";
  const otherTitle = "Linux Study Group";
  await mockGroupSwitcher(page, [
    { id: namedGroupId, title },
    { id: unnamedGroupId, title: otherTitle }
  ]);
  await page.goto(`/groups?group=${namedGroupId}`);
  const trigger = page.getByRole("button", { name: "当前群" });
  await expect(trigger).toHaveText(title);
  await expect(page.locator("[data-groups-page]")).not.toContainText(/-100\d+/);
  await expect(page.locator("[data-group-row]").first().getByRole("link")).toHaveAccessibleName(new RegExp(title));
  await expect(page.locator("[data-group-row]").first().getByRole("link")).not.toHaveAccessibleName(/-100\d+/);

  await trigger.click();
  await expect(page.getByRole("listbox")).not.toContainText(/-100\d+/);
  await page.getByRole("option", { name: otherTitle, exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/groups\\?group=${unnamedGroupId}$`));
  await expect(trigger).toHaveText(otherTitle);
  await expect(trigger).not.toContainText(/-100\d+/);
  await expect(page.locator("[data-group-row][data-selected]").getByRole("heading")).toHaveText(otherTitle);
  await expect(page.locator("[data-groups-page]")).not.toContainText(/-100\d+/);
});

test("group switcher renders angle brackets and emoji as text", async ({ page }) => {
  const title = "<img src=x onerror=alert('owned')> 管理群 🧪";
  await mockGroupSwitcher(page, [{ id: namedGroupId, title }]);
  await page.goto(`/groups?group=${namedGroupId}`);

  const switcher = page.locator("[data-group-switcher]");
  await expect(page.getByRole("button", { name: "当前群" })).toHaveText(title);
  await expect(switcher.locator("img")).toHaveCount(0);

  await page.getByRole("button", { name: "当前群" }).click();
  await expect(page.getByRole("option", { name: title, exact: true })).toBeVisible();
  await expect(switcher.locator("img")).toHaveCount(0);
});
