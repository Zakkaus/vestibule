import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1001163306055";
const selectedGroupTitle = "Gentoo-zh Community";

type Role = "manager" | "operator";
type NavigationSection = Readonly<{
  id: string | null;
  paths: readonly string[];
}>;

const operatorSections: readonly NavigationSection[] = [
  { id: "daily", paths: ["/home", "/queue", "/audit"] },
  { id: "verification", paths: ["/verification", "/questions", "/bypass"] },
  { id: "group", paths: ["/groups", "/moderation", "/messages"] },
  { id: "content", paths: ["/feeds"] },
  { id: "observe", paths: ["/stats", "/diagnostics"] },
  { id: "console", paths: ["/version", "/capabilities", "/preferences"] }
];

async function fulfillJSON(route: Route, body: unknown): Promise<void> {
  await route.fulfill({
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockNavigationTransport(page: Page, role: Role): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: "741928306", role },
        expires_at: "2026-09-04T02:00:00Z",
        csrf_token: "navigation-csrf"
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID, title: selectedGroupTitle }] });
      return;
    }

    if (path === "/api/instance" && request.method() === "GET") {
      await fulfillJSON(route, { bot_username: "example_bot" });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function navigationSections(page: Page, root: string): Promise<readonly NavigationSection[]> {
  const groups = page.locator(`${root} [data-navigation-group]`);
  const sections: NavigationSection[] = [];
  for (let index = 0; index < await groups.count(); index++) {
    const group = groups.nth(index);
    const id = await group.getAttribute("data-navigation-group");
    if (await group.getAttribute("aria-expanded") === "false") await group.click();
    await expect(group).toHaveAttribute("aria-expanded", "true");
    const paths = await page.locator(`${root} nav a[href]`).evaluateAll((links) =>
      links.map((link) => new URL((link as HTMLAnchorElement).href).pathname)
    );
    sections.push({ id, paths });
  }
  return sections;
}

test("navigation groups follow the console responsibility map", async ({ page }) => {
  await mockNavigationTransport(page, "operator");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();

  const sections = await navigationSections(page, ".console-sidebar");
  expect(sections).toEqual(operatorSections);
  expect(sections.every((section) => section.paths.length > 0)).toBe(true);
});

test("group titles replace transport identifiers", async ({ page }) => {
  await mockNavigationTransport(page, "operator");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();

  await page.locator("[data-group-switcher]").getByRole("button").click();
  await expect(page.getByRole("option", { name: selectedGroupTitle })).toBeVisible();
  await expect(page.getByRole("heading", { name: selectedGroupTitle })).toBeVisible();
  await expect(page.locator("[data-groups-page]")).not.toContainText(/-100\d+/);
  await expect(page.getByRole("listbox")).not.toContainText(/-100\d+/);
  await expect(page.locator("[data-group-switcher]")).not.toContainText(/-100\d+/);
});

test("capability filtering leaves no empty navigation section", async ({ page }) => {
  await mockNavigationTransport(page, "manager");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();

  const sections = await navigationSections(page, ".console-sidebar");
  expect(sections).toEqual([
    ...operatorSections.slice(0, -1),
    { id: "console", paths: ["/capabilities", "/preferences"] }
  ]);
  expect(sections.every((section) => section.paths.length > 0)).toBe(true);
});


test.describe("mobile navigation", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test("opens all sections and returns keyboard focus when dismissed", async ({ page }) => {
    await mockNavigationTransport(page, "operator");
    await page.goto("/groups");
    await expect(page.locator("[data-groups-source='api']")).toBeVisible();

    const trigger = page.locator("[data-mobile-navigation]").getByRole("button");
    await trigger.focus();
    await page.keyboard.press("Enter");
    const panel = page.getByRole("dialog");
    await expect(panel).toBeVisible();
    expect(await navigationSections(page, ".console-mobile-panel")).toEqual(operatorSections);
    const box = await panel.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(390);

    await page.keyboard.press("Escape");
    await expect(panel).toBeHidden();
    await expect(trigger).toBeFocused();
  });
});
