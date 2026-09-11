import { expect, test } from "@playwright/test";
import { mockSpectrumTransport, openSpectrumRoute, selectedGroupID } from "./spectrum-fixtures";

const destinationIcons = [
  "layoutDashboard", "inbox", "clipboardList",
  "shieldCheck", "circleHelp", "shieldOff",
  "usersRound", "shieldAlert", "messagesSquare",
  "rss",
  "chartNoAxesCombined", "activity",
  "refreshCw", "slidersHorizontal", "settings"
];

test("navigation rows and section headers keep the side nav's own rhythm in the widest locale", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");

  const geometry = await page.locator(".console-sidebar").evaluate((sidebar) => {
    const links = [...sidebar.querySelectorAll<HTMLAnchorElement>("[data-navigation-item]")];
    const headers = [...sidebar.querySelectorAll<HTMLElement>("[data-navigation-group]")];
    if (links.length !== 15 || headers.length !== 6) throw new Error("Navigation geometry is incomplete");

    const linkBoxes = links.map((link) => {
      const box = link.getBoundingClientRect();
      const css = getComputedStyle(link);
      return {
        item: link.dataset.navigationItem,
        height: box.height,
        fontSize: Number.parseFloat(css.fontSize),
        top: box.top,
        bottom: box.bottom
      };
    });
    const headerBoxes = headers.map((header) => {
      const box = header.getBoundingClientRect();
      return { id: header.dataset.navigationGroup, top: box.top, bottom: box.bottom, fontSize: Number.parseFloat(getComputedStyle(header).fontSize) };
    });
    // Gaps between two rows of one section, and from a header to its first row.
    const rowGaps = [];
    const headerGaps = [];
    for (const header of headerBoxes) {
      const rows = linkBoxes.filter((link) => link.top > header.bottom &&
        !headerBoxes.some((other) => other.top > header.bottom && other.top < link.top));
      headerGaps.push(rows[0]!.top - header.bottom);
      for (let index = 1; index < rows.length; index++) rowGaps.push(rows[index]!.top - rows[index - 1]!.bottom);
    }
    return {
      sidebarWidth: sidebar.getBoundingClientRect().width,
      linkBoxes,
      headerBoxes,
      rowGaps,
      headerGaps
    };
  });

  expect(geometry.sidebarWidth).toBe(280);
  // Every English label fits one 32px row: the sidebar is sized for the longest one.
  for (const link of geometry.linkBoxes) {
    expect(Math.abs(link.height - 32), link.item).toBeLessThanOrEqual(1);
    expect(Math.abs(link.fontSize - 14), link.item).toBeLessThanOrEqual(0.1);
  }
  for (const header of geometry.headerBoxes) {
    expect(Math.abs(header.fontSize - 12), header.id!).toBeLessThanOrEqual(0.1);
  }
  for (const gap of geometry.rowGaps) expect(Math.abs(gap - 6)).toBeLessThanOrEqual(1);
  for (const gap of geometry.headerGaps) expect(Math.abs(gap - 8)).toBeLessThanOrEqual(1);
});

test("home content fits one screen at both suite heights and scrolls only when the window is short", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");

  const measurePanel = () => page.locator(".console-content").evaluate((panel) => {
    panel.scrollTo(0, panel.scrollHeight);
    const sections = [...panel.querySelectorAll<HTMLElement>("[data-home-section]")].filter((section) =>
      section.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }) &&
      section.getBoundingClientRect().height > 0
    );
    const last = sections.at(-1);
    if (!last) throw new Error("Home sections are incomplete");
    const panelBox = panel.getBoundingClientRect();
    const lastBottom = Math.max(...sections.map((section) => section.getBoundingClientRect().bottom));
    return {
      bottomGap: panelBox.bottom - lastBottom,
      paddingBottom: Number.parseFloat(getComputedStyle(panel).paddingBlockEnd),
      overflowY: getComputedStyle(panel).overflowY,
      scrollable: panel.scrollHeight > panel.clientHeight,
      horizontalOverflow: panel.scrollWidth > panel.clientWidth + 1
    };
  });

  for (const height of [720, 900]) {
    await page.setViewportSize({ width: 1280, height });
    const desktop = await measurePanel();
    expect(desktop.scrollable, `${height}px`).toBe(false);
    expect(desktop.horizontalOverflow, `${height}px`).toBe(false);
    // Nothing is clipped: the last section ends above the panel's own bottom padding.
    expect(desktop.bottomGap, `${height}px`).toBeGreaterThanOrEqual(desktop.paddingBottom - 1);
  }

  await page.setViewportSize({ width: 420, height: 420 });
  const narrow = await measurePanel();
  expect(narrow.overflowY).toBe("auto");
  expect(narrow.scrollable).toBe(true);
  expect(narrow.horizontalOverflow).toBe(false);
  expect(narrow.bottomGap).toBeGreaterThanOrEqual(narrow.paddingBottom - 1);
  await expect(page.locator("[data-home-section]:visible").last()).toBeVisible();
});

test("navigation preserves route hooks, icons, and the side nav's arrow-key entry", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");

  const nav = page.locator(".console-sidebar .console-navigation [role='treegrid']");
  await expect(nav).toHaveAttribute("aria-label", /.+/);
  await expect(page.locator(".console-content")).toHaveCount(1);
  await expect(nav.locator("[data-navigation-group]")).toHaveCount(6);
  await expect(nav.locator('a[aria-current="page"]')).toHaveAttribute("data-navigation-item", "/home");
  expect(await nav.locator("[data-navigation-item] [data-icon-name]").evaluateAll((icons) =>
    icons.map((icon) => icon.getAttribute("data-icon-name"))
  )).toEqual(destinationIcons);

  // Tab lands on the current destination; arrows walk the rows across section
  // boundaries; the focus ring is the library's, drawn on the row; Enter follows.
  await page.locator(".console-brand a").focus();
  await page.keyboard.press("Tab");
  await expect(nav.locator('[data-navigation-item="/home"]')).toBeFocused();
  for (let step = 0; step < 3; step++) await page.keyboard.press("ArrowDown");
  const link = nav.locator('[data-navigation-item="/verification"]');
  await expect(link).toBeFocused();
  const ringOf = (item: string) => nav.locator(`[data-navigation-item="${item}"]`).locator("xpath=ancestor::*[@role='gridcell']/div[1]");
  await expect(ringOf("/verification")).toHaveCSS("outline-style", "solid");
  await expect(ringOf("/queue")).toHaveCSS("outline-style", "none");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL((url) =>
    url.pathname === "/verification" && url.searchParams.get("group") === selectedGroupID
  );
});
