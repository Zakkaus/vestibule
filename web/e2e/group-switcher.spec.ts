import { expect, test, type Page, type Route } from "@playwright/test";

const namedGroupId = "-1004237282609";
const unnamedGroupId = "-1001834029912";

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

test("group switcher prefers a title and falls back to the group ID", async ({ page }) => {
  const title = "维护者讨论群";
  await mockGroupSwitcher(page, [
    { id: namedGroupId, title },
    { id: unnamedGroupId }
  ]);
  await page.goto(`/groups?group=${namedGroupId}`);

  const trigger = page.getByRole("button", { name: "当前群" });
  await expect(trigger).toHaveText(title);
  await trigger.click();
  await expect(page.getByRole("option", { name: title, exact: true })).toBeVisible();

  await page.getByRole("option", { name: unnamedGroupId, exact: true }).click();
  await expect(trigger).toHaveText(unnamedGroupId);
  await expect(page).toHaveURL(new RegExp(`/groups\\?group=${unnamedGroupId}$`));
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
