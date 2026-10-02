import { expect, test, type Page, type Route } from "@playwright/test";
import { mockSpectrumTransport } from "./spectrum-fixtures";

const selectedGroupID = "-1009000010001";
const selectedGroupTitle = "Gentoo-zh Community";

type Role = "manager" | "operator";
type NavigationSection = Readonly<{
  id: string | null;
  paths: readonly string[];
}>;

const operatorSections: readonly NavigationSection[] = [
  { id: "daily", paths: ["/home", "/queue", "/audit", "/stats"] },
  { id: "verification", paths: ["/verification", "/questions", "/bypass"] },
  { id: "group", paths: ["/groups", "/moderation", "/messages", "/feeds"] },
  { id: "console", paths: ["/diagnostics", "/version", "/capabilities", "/preferences"] }
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
        is_owner: false,
        csrf_token: "navigation-csrf"
      });
      return;
    }

    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID, title: selectedGroupTitle, owner: null, administrators: [], administrators_status: "unavailable" }] });
      return;
    }

    if (path === "/api/instance" && request.method() === "GET") {
      await fulfillJSON(route, { bot_username: "example_bot" });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

// Sections are flat: read what is on screen. Nothing is expanded, because nothing is
// collapsed. The section a destination belongs to is the header above it.
async function navigationSections(page: Page, root: string): Promise<readonly NavigationSection[]> {
  await expect(page.locator(`${root} [data-navigation-item]`).first()).toBeVisible();
  return page.locator(root).evaluate((sidebar) => {
    const sections: { id: string; paths: string[] }[] = [];
    for (const node of sidebar.querySelectorAll("[data-navigation-group], [data-navigation-item]")) {
      const group = node.getAttribute("data-navigation-group");
      if (group !== null) {
        sections.push({ id: group, paths: [] });
        continue;
      }
      const link = node.closest("a[href]") ?? node.querySelector("a[href]") ?? node;
      const href = link.getAttribute("href");
      if (href !== null && sections.length > 0) {
        sections[sections.length - 1]!.paths.push(new URL(href, location.href).pathname);
      }
    }
    return sections;
  });
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
    { id: "console", paths: ["/diagnostics", "/capabilities", "/preferences"] }
  ]);
  expect(sections.every((section) => section.paths.length > 0)).toBe(true);
});


test.describe("mobile navigation", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test("bottom hubs expose the current hub pages and navigate by keyboard", async ({ page }) => {
    await mockSpectrumTransport(page, { role: "operator" });
    await page.goto("/groups");
    await expect(page.locator("[data-groups-source='api']")).toBeVisible();
    const bar = page.locator("[data-hub-bar]");
    await expect(bar.getByRole("link")).toHaveCount(4);
    await expect(page.locator("[data-hub-pages] [data-navigation-item]")).toHaveCount(4);
    const consoleHub = bar.locator('[data-hub="console"]');
    await consoleHub.focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(url => url.pathname === "/diagnostics");
    await expect(page.locator("[data-hub-pages] [data-navigation-item]")).toHaveCount(4);
    await expect(page.locator("h1")).toBeFocused();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });
});

test("every destination is visible without opening a section", async ({ page }) => {
  await mockNavigationTransport(page, "operator");
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/home");
  await expect(page.locator("[data-home-page], [data-console-page]").first()).toBeVisible();

  const sidebar = page.locator(".console-sidebar");
  const items = sidebar.locator("[data-navigation-item]");
  const headers = sidebar.locator("[data-navigation-group]");
  // Four expanded hubs expose every authorized destination.
  await expect(items).toHaveCount(operatorSections.reduce((total, section) => total + section.paths.length, 0));
  await expect(headers).toHaveCount(operatorSections.length);
  for (const item of await items.all()) await expect(item).toBeVisible();
  for (const header of await headers.all()) await expect(header).toBeVisible();

  // The current destination is marked by the library's own indicator, not by a rule here.
  // The row and its link both carry the attribute, so name the one that is a destination.
  const current = sidebar.locator("[data-navigation-item][data-current]");
  await expect(current).toHaveCount(1);
  await expect(current).toHaveAttribute("data-navigation-item", "/home");
});

test("the content panel reaches the bottom of the window on the home page", async ({ page }) => {
  await mockNavigationTransport(page, "operator");
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/home");
  await expect(page.locator("[data-home-page], [data-console-page]").first()).toBeVisible();

  const gap = await page.locator(".console-content").evaluate(
    (element) => window.innerHeight - element.getBoundingClientRect().bottom
  );
  expect(gap).toBeLessThanOrEqual(1);
});
