import { expect, test, type Page, type Route } from "@playwright/test";

const groupID = "-1009000002412";
const unavailableGroupID = "-1009000002413";

const owner = {
  id: "9000002414",
  first_name: "Mira",
  last_name: "Owner",
  username: "mira_owner"
} as const;

const creatorPermissions = {
  can_manage_chat: true,
  can_delete_messages: true,
  can_manage_video_chats: true,
  can_restrict_members: true,
  can_promote_members: true,
  can_change_info: true,
  can_invite_users: true,
  can_post_stories: true,
  can_edit_stories: true,
  can_delete_stories: true,
  can_post_messages: null,
  can_edit_messages: null,
  can_pin_messages: true,
  can_manage_topics: true
} as const;

const administratorWithRights = {
  can_manage_chat: true,
  can_delete_messages: true,
  can_manage_video_chats: false,
  can_restrict_members: true,
  can_promote_members: false,
  can_change_info: true,
  can_invite_users: false,
  can_post_stories: false,
  can_edit_stories: false,
  can_delete_stories: false,
  can_post_messages: null,
  can_edit_messages: null,
  can_pin_messages: true,
  can_manage_topics: false
} as const;

const administratorWithoutRights = {
  can_manage_chat: false,
  can_delete_messages: false,
  can_manage_video_chats: false,
  can_restrict_members: false,
  can_promote_members: false,
  can_change_info: false,
  can_invite_users: false,
  can_post_stories: false,
  can_edit_stories: false,
  can_delete_stories: false,
  can_post_messages: null,
  can_edit_messages: null,
  can_pin_messages: false,
  can_manage_topics: false
} as const;

const availableChat = {
  id: groupID,
  title: "Owner display group",
  owner,
  administrators_status: "available",
  administrators: [
    {
      user: owner,
      status: "creator",
      permissions: creatorPermissions
    },
    {
      user: {
        id: "9000002415",
        first_name: "Ada",
        last_name: "WithRights",
        username: "ada_rights"
      },
      status: "administrator",
      permissions: administratorWithRights
    },
    {
      user: {
        id: "9000002416",
        first_name: "Bea",
        last_name: "WithoutRights",
        username: "bea_no_rights"
      },
      status: "administrator",
      permissions: administratorWithoutRights
    }
  ]
} as const;

const unavailableChat = {
  id: unavailableGroupID,
  title: "Unreadable owner group",
  owner: null,
  administrators_status: "unavailable",
  administrators: []
} as const;

async function fulfillJSON(route: Route, body: unknown): Promise<void> {
  await route.fulfill({
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockGroups(page: Page, chats: readonly unknown[]): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: "9000002417", role: "manager" },
        expires_at: "2030-09-04T12:00:00Z",
        is_owner: false,
        csrf_token: "group-owner-csrf"
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

test("group cards show the Telegram creator and every applicable administrator right", async ({ page }) => {
  await mockGroups(page, [availableChat]);
  await page.goto(`/groups?group=${groupID}`);

  const row = page.locator(`[data-group-row][data-group-chat-id="${groupID}"]`);
  await expect(row).toBeVisible();
  const ownerCard = row.locator("[data-group-owner]");
  await expect(ownerCard).toContainText("Mira");
  await expect(ownerCard).toContainText("Owner");
  await expect(ownerCard).toContainText(owner.id);

  const administrators = row.locator("[data-group-administrators]");
  await expect(administrators).toBeVisible();
  await expect(administrators.locator("[data-group-administrator]")).toHaveCount(3);

  const creator = administrators.locator(`[data-group-administrator][data-user-id="${owner.id}"]`);
  await expect(creator).toContainText("Mira");
  await expect(creator.locator("[data-administrator-permission]")).toHaveCount(12);
  await expect(
    creator.locator('[data-administrator-permission][data-permission-key="can_restrict_members"]')
  ).toHaveAttribute("data-permission-state", "granted");

  const withRights = administrators.locator('[data-group-administrator][data-user-id="9000002415"]');
  await expect(withRights).toContainText("Ada");
  await expect(
    withRights.locator('[data-administrator-permission][data-permission-key="can_restrict_members"]')
  ).toHaveAttribute("data-permission-state", "granted");
  await expect(
    withRights.locator('[data-administrator-permission][data-permission-key="can_invite_users"]')
  ).toHaveAttribute("data-permission-state", "denied");

  const withoutRights = administrators.locator('[data-group-administrator][data-user-id="9000002416"]');
  await expect(withoutRights).toContainText("Bea");
  await expect(
    withoutRights.locator('[data-administrator-permission][data-permission-key="can_restrict_members"]')
  ).toHaveAttribute("data-permission-state", "denied");
  await expect(
    withoutRights.locator('[data-administrator-permission][data-permission-key="can_delete_messages"]')
  ).toHaveAttribute("data-permission-state", "denied");

  for (const administrator of [creator, withRights, withoutRights]) {
    await expect(
      administrator.locator('[data-administrator-permission][data-permission-key="can_post_messages"]')
    ).toHaveCount(0);
    await expect(
      administrator.locator('[data-administrator-permission][data-permission-key="can_edit_messages"]')
    ).toHaveCount(0);
  }

  await expect(page.locator("body")).not.toContainText(/改绑|改綁|rebind|transfer ownership/i);
});

test("an unavailable Telegram administrator lookup does not invent an owner", async ({ page }) => {
  await mockGroups(page, [unavailableChat]);
  await page.goto(`/groups?group=${unavailableGroupID}`);

  const row = page.locator(`[data-group-row][data-group-chat-id="${unavailableGroupID}"]`);
  await expect(row).toBeVisible();
  await expect(row.locator("[data-group-owner]")).toHaveCount(0);
  await expect(row.locator('[data-group-administrators-status="unavailable"]')).toBeVisible();
  await expect(row.locator("[data-group-administrator]")).toHaveCount(0);
  await expect(row).not.toContainText(owner.id);
  await expect(row).not.toContainText(/改绑|改綁|rebind|transfer ownership/i);
});
