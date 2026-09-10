import { expect, test } from "@playwright/test";
import { mockSpectrumTransport, openSpectrumRoute, selectedGroupID } from "./spectrum-fixtures";

test("navigation uses docs-like Spectrum type and spacing tokens", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");

  const geometry = await page.locator(".console-sidebar").evaluate((sidebar) => {
    const nav = sidebar.querySelector("nav");
    const links = [...sidebar.querySelectorAll<HTMLAnchorElement>("nav a[href]")].filter((link) =>
      link.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }) &&
      link.getBoundingClientRect().height > 0
    );
    const sections = [...sidebar.querySelectorAll<HTMLElement>("[data-navigation-group]")].map((group) =>
      group.closest<HTMLElement>("[data-navigation-section], [role='treeitem']") ??
      group.parentElement ??
      group
    );
    if (!nav || links.length < 2 || sections.length < 2) throw new Error("Navigation geometry is incomplete");

    const linkBoxes = links.map((link) => {
      const box = link.getBoundingClientRect();
      const css = getComputedStyle(link);
      return {
        height: box.height,
        fontSize: Number.parseFloat(css.fontSize),
        lineHeight: Number.parseFloat(css.lineHeight),
        top: box.top,
        bottom: box.bottom
      };
    });
    const sectionBoxes = sections.map((section) => {
      const box = section.getBoundingClientRect();
      return { top: box.top, bottom: box.bottom };
    });
    return {
      sidebarWidth: sidebar.getBoundingClientRect().width,
      linkBoxes,
      itemGap: linkBoxes[1].top - linkBoxes[0].bottom,
      groupGap: sectionBoxes[1].top - sectionBoxes[0].bottom
    };
  });

  expect(geometry.sidebarWidth).toBeCloseTo(224, 0);
  expect(geometry.linkBoxes.every(({ height }) => Math.abs(height - 32) <= 1)).toBe(true);
  expect(geometry.linkBoxes.every(({ fontSize }) => Math.abs(fontSize - 14) <= 0.1)).toBe(true);
  expect(geometry.linkBoxes.every(({ lineHeight }) => Math.abs(lineHeight - 32) <= 1)).toBe(true);
  expect(geometry.itemGap).toBeGreaterThanOrEqual(5);
  expect(geometry.itemGap).toBeLessThanOrEqual(7);
  expect(geometry.groupGap).toBeGreaterThanOrEqual(63);
  expect(geometry.groupGap).toBeLessThanOrEqual(65);
});
test("home content stays intrinsic while short narrow panels remain scrollable", async ({ page }) => {
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
      sectionGap: Number.parseFloat(getComputedStyle(last.parentElement!).rowGap),
      overflowY: getComputedStyle(panel).overflowY,
      scrollable: panel.scrollHeight > panel.clientHeight,
      horizontalOverflow: panel.scrollWidth > panel.clientWidth + 1
    };
  });

  const desktop = await measurePanel();
  expect(desktop.bottomGap).toBeGreaterThanOrEqual(-1);
  expect(desktop.bottomGap).toBeLessThanOrEqual(desktop.sectionGap + 0.5);

  await page.setViewportSize({ width: 420, height: 420 });
  const narrow = await measurePanel();
  expect(narrow.overflowY).toBe("auto");
  expect(narrow.scrollable).toBe(true);
  expect(narrow.horizontalOverflow).toBe(false);
  expect(narrow.bottomGap).toBeGreaterThanOrEqual(-1);
  expect(narrow.bottomGap).toBeLessThanOrEqual(narrow.sectionGap + 0.5);
  await expect(page.locator("[data-home-section]:visible").last()).toBeVisible();
});

test("navigation preserves route hooks, icons, and native accordion keyboard entry", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");

  const nav = page.locator(".console-sidebar nav");
  await expect(nav).toHaveAttribute("aria-label");
  await expect(page.locator(".console-content")).toHaveCount(1);
  await expect(nav.locator("[data-navigation-group]")).toHaveCount(6);
  await expect(nav.locator('a[aria-current="page"]')).toHaveAttribute("data-navigation-item", "/home");
  expect(await nav.locator("a[href]:visible [data-icon-name]").evaluateAll((icons) =>
    icons.map((icon) => icon.getAttribute("data-icon-name"))
  )).toEqual(["layoutDashboard", "inbox", "clipboardList"]);

  const verification = nav.locator('[data-navigation-group="verification"]');
  await verification.focus();
  await page.keyboard.press("Space");
  await expect(verification).toHaveAttribute("aria-expanded", "true");

  const link = nav.locator('a[href^="/verification"]');
  await expect(link).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(link).toBeFocused();
  await expect(link).toHaveCSS("outline-style", "solid");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL((url) =>
    url.pathname === "/verification" && url.searchParams.get("group") === selectedGroupID
  );
});
