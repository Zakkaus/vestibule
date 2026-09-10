import { expect, test, type Page } from "@playwright/test";

type EntryResponses = Readonly<{
  session: { status: number; body: unknown };
  instance: { status?: number; body: unknown };
  chats?: { status?: number; body: unknown };
}>;

async function mockProductionEntry(page: Page, responses: EntryResponses): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const response =
      path === "/api/session"
        ? responses.session
        : path === "/api/instance"
          ? responses.instance
          : path === "/api/chats" && responses.chats
            ? responses.chats
            : undefined;

    if (!response) {
      throw new Error(`Unexpected API request: ${request.method()} ${path}`);
    }

    await route.fulfill({
      status: response.status ?? 200,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(response.body)
    });
  });
}

test("production ignores a fixture query after an unauthenticated response", async ({ page }) => {
  await mockProductionEntry(page, {
    session: {
      status: 401,
      body: { error: { code: "authentication_invalid" } }
    },
    instance: { body: { bot_username: "actual_instance_bot" } }
  });

  await page.goto("/missing?state=no-groups");

  const entry = page.locator("[data-entry-page]");
  await expect(entry).toHaveAttribute("data-entry-state", "no-session");
  await expect(entry).not.toContainText("741928306");
});

test("production recognises an unclaimed instance even with a fixture query", async ({ page }) => {
  await mockProductionEntry(page, {
    session: {
      status: 401,
      body: { error: { code: "authentication_invalid" } }
    },
    instance: { body: { bot_username: "" } }
  });

  await page.goto("/missing?state=no-groups");

  const entry = page.locator("[data-entry-page]");
  await expect(entry).toHaveAttribute("data-entry-state", "unclaimed");
});

test("production keeps the real account identity for a valid session without groups", async ({ page }) => {
  const accountId = "987654321";
  await mockProductionEntry(page, {
    session: {
      status: 200,
      body: {
        subject: { telegram_id: accountId, role: "manager" },
        expires_at: "2030-09-04T12:00:00Z",
        csrf_token: "production-entry-csrf"
      }
    },
    instance: { body: { bot_username: "" } },
    chats: { body: { chats: [] } }
  });

  await page.goto("/missing?state=expired");

  const entry = page.locator("[data-entry-page]");
  await expect(entry).toHaveAttribute("data-entry-state", "no-groups");
  await expect(entry).toContainText(accountId);
  await expect(entry).not.toContainText("741928306");
});
