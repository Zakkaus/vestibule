import { expect, test, type Page, type Route } from "@playwright/test";

const ownerID = "9000000101";
const groupID = "-1009000010101";

const initialLimits = {
  timeout_seconds: 600,
  ban_seconds: 86400,
  mute_seconds: 3600,
  lookup_ttl_seconds: 180,
  verify_retry_seconds: 180,
  verify_max_fails: 3,
  warn_limit: 3,
  private_query_per_min: 3,
  questions: 10,
  fallback_questions: 10,
  channel_whitelist: 5,
  trusted_member_group_ids: 2,
  known_chat_ids: 4
} as const;

const updatedLimits = { ...initialLimits, timeout_seconds: 1500 };

const staleViolation = {
  chat_id: groupID,
  field: "timeout_seconds",
  value: 1200,
  limit: initialLimits.timeout_seconds
} as const;

type OwnerTransportOptions = Readonly<{
  isOwner: boolean;
  noGroups?: boolean;
  onPatch?: (route: Route) => Promise<void>;
}>;

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockOwnerTransport(page: Page, options: OwnerTransportOptions): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: ownerID, role: "manager" },
        expires_at: "2030-09-04T02:00:00Z",
        csrf_token: "owner-csrf",
        is_owner: options.isOwner
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, {
        chats: !options.noGroups
          ? [{
              id: groupID,
              title: "Limits group",
              owner: null,
              administrators: [],
              administrators_status: "unavailable"
            }]
          : []
      });
      return;
    }

    if (path === "/api/instance" && request.method() === "GET") {
      await fulfillJSON(route, { bot_username: "example_bot" });
      return;
    }

    if (path === "/api/owner/limits" && request.method() === "GET") {
      if (!options.isOwner) throw new Error("a non-owner must not request owner limits");
      await fulfillJSON(route, {
        revision: 7,
        limits: initialLimits,
        violations: [staleViolation]
      });
      return;
    }

    if (path === "/api/owner/limits" && request.method() === "PATCH") {
      if (!options.isOwner) throw new Error("a non-owner must not patch owner limits");
      await options.onPatch?.(route);
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

test("owner navigation opens limits, marks stale groups, and saves a new limit", async ({ page }) => {
  let patchBody: unknown;
  let patchCSRF: string | undefined;
  await mockOwnerTransport(page, {
    isOwner: true,
    onPatch: async (route) => {
      patchBody = route.request().postDataJSON();
      patchCSRF = route.request().headers()["x-csrf-token"];
      await fulfillJSON(route, {
        revision: 8,
        limits: updatedLimits,
        violations: []
      });
    }
  });

  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/groups");
  const ownerNavigation = page.locator("[data-owner-navigation]");
  await expect(ownerNavigation).toHaveAttribute("href", "/owner");
  await ownerNavigation.click();
  await expect(page).toHaveURL(/\/owner$/);

  await expect(page.locator("[data-owner-page]")).toBeVisible();
  await expect(page.locator("[data-owner-limits-form]")).toBeVisible();
  await expect(page.locator(`[data-owner-limit-violation="${groupID}:timeout_seconds"]`)).toContainText(groupID);

  await page.locator('[data-owner-limit-input="timeout_seconds"]').fill("1500");
  await page.locator("[data-owner-limits-save]").click();
  await expect.poll(() => patchBody).toEqual({
    expected_revision: 7,
    changes: { timeout_seconds: 1500 }
  });
  expect(patchCSRF).toBe("owner-csrf");
  await expect(page.locator("[data-owner-limits-feedback]")).toBeVisible();
  await expect(page.locator("[data-owner-limit-violation]")).toHaveCount(0);
});

test("non-owners cannot see or request owner limits", async ({ page }) => {
  await mockOwnerTransport(page, { isOwner: false });

  await page.goto(`/owner?group=${groupID}`);
  await expect(page.locator("[data-group-switcher]")).toContainText("Limits group");
  await expect(page.locator("[data-owner-limits-form]")).toHaveCount(0);
  await expect(page.locator("[data-owner-navigation]")).toHaveCount(0);
});

test("an owner without any managed groups can edit instance caps", async ({ page }) => {
  await mockOwnerTransport(page, { isOwner: true, noGroups: true });
  await page.goto("/owner");
  await expect(page.locator("[data-owner-limits-form]")).toBeVisible();
  await page.locator('[data-owner-limit-input="questions"]').fill("12");
  await expect(page.locator("[data-owner-limits-save]")).toBeEnabled();
});
