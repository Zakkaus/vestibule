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
  return page.locator(`${root} [data-navigation-group]`).evaluateAll((elements) =>
    elements.map((element) => ({
      id: element.getAttribute("data-navigation-group"),
      paths: [...element.querySelectorAll("a[href]")].map(
        (link) => new URL((link as HTMLAnchorElement).href).pathname
      )
    }))
  );
}

test("navigation groups follow the console responsibility map", async ({ page }) => {
  await mockNavigationTransport(page, "operator");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();

  const sections = await navigationSections(page, ".shell-aside");
  expect(sections).toEqual(operatorSections);
  expect(sections.every((section) => section.paths.length > 0)).toBe(true);
});

test("group titles replace transport identifiers", async ({ page }) => {
  await mockNavigationTransport(page, "operator");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();

  await page.locator("[data-group-switcher] [data-slot='select-trigger']").click();
  await expect(page.getByRole("option", { name: selectedGroupTitle })).toBeVisible();
  await expect(page.getByRole("heading", { name: selectedGroupTitle })).toBeVisible();
});

test("capability filtering leaves no empty navigation section", async ({ page }) => {
  await mockNavigationTransport(page, "manager");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();

  const sections = await navigationSections(page, ".shell-aside");
  expect(sections).toEqual([
    ...operatorSections.slice(0, -1),
    { id: "console", paths: ["/capabilities", "/preferences"] }
  ]);
  expect(sections.every((section) => section.paths.length > 0)).toBe(true);
});

test("sidebar navigation uses one spacing hierarchy", async ({ page }) => {
  await mockNavigationTransport(page, "operator");
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto("/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();

  const spacing = await page.locator(".shell-aside .nav").evaluate((navigation) => {
    const daily = navigation.querySelector<HTMLElement>("[data-navigation-group='daily']");
    const verification = navigation.querySelector<HTMLElement>(
      "[data-navigation-group='verification']"
    );
    const label = daily?.querySelector<HTMLElement>(".nav-label");
    const items = daily ? [...daily.querySelectorAll<HTMLElement>(".nav-item")] : [];
    const nextLabel = verification?.querySelector<HTMLElement>(".nav-label");

    if (!daily || !verification || !label || !nextLabel || items.length < 2) {
      throw new Error("Navigation spacing targets are missing");
    }

    const labelRect = label.getBoundingClientRect();
    const firstItem = items[0].getBoundingClientRect();
    const secondItem = items[1].getBoundingClientRect();
    const lastItem = items.at(-1)?.getBoundingClientRect();
    const nextLabelRect = nextLabel.getBoundingClientRect();
    if (!lastItem) {
      throw new Error("Daily navigation group has no final item");
    }

    return {
      groupGap: window.getComputedStyle(navigation).rowGap,
      itemGap: window.getComputedStyle(daily).rowGap,
      labelMarginEnd: window.getComputedStyle(label).marginBlockEnd,
      labelToItem: firstItem.top - labelRect.bottom,
      itemToItem: secondItem.top - firstItem.bottom,
      groupToGroup: nextLabelRect.top - lastItem.bottom
    };
  });

  expect(spacing.groupGap).toBe("16px");
  expect(spacing.itemGap).toBe("8px");
  expect(spacing.labelMarginEnd).toBe("0px");
  expect(spacing.labelToItem).toBeCloseTo(8, 1);
  expect(spacing.itemToItem).toBeCloseTo(8, 1);
  expect(spacing.groupToGroup).toBeCloseTo(16, 1);
});

test.describe("mobile navigation", () => {
  test.use({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });

  test("joins the panel to its trigger with compact touch navigation", async ({ page }) => {
    await mockNavigationTransport(page, "operator");
    await page.goto("/groups");
    await expect(page.locator("[data-groups-source='api']")).toBeVisible();

    const trigger = page.locator("[data-mobile-navigation] summary");
    await expect(trigger).toBeVisible();
    await trigger.click();

    const panel = page.locator("[data-mobile-navigation][open] nav");
    await expect(panel).toBeVisible();
    expect(await navigationSections(page, "[data-mobile-navigation][open]")).toEqual(operatorSections);

    const geometry = await page.locator("[data-mobile-navigation]").evaluate((details) => {
      const trigger = details.querySelector<HTMLElement>("summary");
      const panel = details.querySelector<HTMLElement>("nav");
      const daily = panel?.querySelector<HTMLElement>("[data-navigation-group='daily']");
      const label = daily?.querySelector<HTMLElement>(".nav-label");
      const items = daily ? [...daily.querySelectorAll<HTMLElement>(".nav-item")] : [];
      if (!trigger || !panel || !label || items.length < 2) {
        throw new Error("Mobile navigation geometry targets are missing");
      }
      const triggerRect = trigger.getBoundingClientRect();
      const panelRect = panel.getBoundingClientRect();
      const labelRect = label.getBoundingClientRect();
      const firstItem = items[0].getBoundingClientRect();
      const secondItem = items[1].getBoundingClientRect();
      const triggerStyle = window.getComputedStyle(trigger);
      const panelStyle = window.getComputedStyle(panel);
      return {
        triggerHeight: triggerRect.height,
        panelOffset: panelRect.top - triggerRect.bottom,
        panelStartOffset: panelRect.left - triggerRect.left,
        panelPadding: `${panelStyle.paddingTop} ${panelStyle.paddingRight} ${panelStyle.paddingBottom} ${panelStyle.paddingLeft}`,
        sectionGap: panelStyle.rowGap,
        itemGap: window.getComputedStyle(daily).rowGap,
        labelToItem: firstItem.top - labelRect.bottom,
        itemToItem: secondItem.top - firstItem.bottom,
        triggerEndRadius: triggerStyle.borderEndStartRadius,
        panelStartRadius: panelStyle.borderStartStartRadius,
        panelStartBorder: panelStyle.borderBlockStartWidth
      };
    });

    expect(geometry.triggerHeight).toBeGreaterThanOrEqual(44);
    expect(geometry.panelOffset).toBeCloseTo(0, 1);
    expect(geometry.panelStartOffset).toBeCloseTo(0, 1);
    expect(geometry.panelPadding).toBe("8px 8px 8px 8px");
    expect(geometry.sectionGap).toBe("12px");
    expect(geometry.itemGap).toBe("4px");
    expect(geometry.labelToItem).toBeCloseTo(4, 1);
    expect(geometry.itemToItem).toBeCloseTo(4, 1);
    expect(geometry.triggerEndRadius).toBe("0px");
    expect(geometry.panelStartRadius).toBe("0px");
    expect(geometry.panelStartBorder).toBe("0px");
  });
});
