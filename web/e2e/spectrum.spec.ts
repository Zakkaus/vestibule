import { expect, test, type Page } from "@playwright/test";

import { selectAppOption } from "./app-select";
import { horizontalGeometry } from "./render-gate-audits";

import {
  chartDays,
  installSpectrumClock,
  managerNavigationGroups,
  mockSpectrumTransport,
  openSpectrumRoute,
  operatorNavigationGroups,
  selectedGroupID
} from "./spectrum-fixtures";

const operatorPaths = operatorNavigationGroups.flatMap((group) => [...group.paths]);
const managerPaths = managerNavigationGroups.flatMap((group) => [...group.paths]);


async function waitForHome(page: Page): Promise<void> {
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
}

async function waitForQueue(page: Page): Promise<void> {
  await expect(page.locator("[data-queue-page]")).toHaveAttribute("data-queue-state", "populated");
}

// Every section of the side nav is open and visible at once; a section is the
// library's row group, headed by our data-navigation-group text.
async function navigationSections(page: Page, root: string) {
  const groups = page.locator(`${root} [role="rowgroup"]`);
  await expect(groups.first()).toBeVisible();
  return groups.evaluateAll((sections) => sections.map((section) => ({
    id: section.querySelector("[data-navigation-group]")?.getAttribute("data-navigation-group") ?? null,
    paths: [...section.querySelectorAll<HTMLAnchorElement>("[data-navigation-item]")].filter((link) =>
      link.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })
    ).map((link) => new URL(link.href).pathname)
  })));
}


async function controlGeometry(page: Page) {
  return page.evaluate(() => {
    const expectedHeight: Record<string, number> = {
      M: 32,
      L: 40,
      XL: matchMedia("not ((hover: hover) and (pointer: fine))").matches ? 60 : 48
    };
    const controls = [...document.querySelectorAll<HTMLElement>("[data-console-control]")]
      .filter((element) => !element.hasAttribute("data-console-card"))
      .filter((element) => element.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }));
    return controls.map((root) => {
      const inner = root.matches("button, input, textarea, select, [role='button'], [role='combobox']")
        ? root
        : root.querySelector<HTMLElement>("button, input, textarea, select, [role='button'], [role='combobox']") ?? root;
      const face = inner.matches("input, textarea")
        ? inner.closest<HTMLElement>('[role="presentation"]') ?? inner
        : inner;
      return {
        size: root.getAttribute("data-control-size"),
        innerHeight: Math.round(face.getBoundingClientRect().height),
        expectedHeight: expectedHeight[root.getAttribute("data-control-size") ?? ""] ?? null,
        innerTag: face.tagName.toLowerCase()
      };
    });
  });
}

async function waitForFonts(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await document.fonts.ready;
    await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  });
}


test("operator navigation exposes all 15 destinations through accessible groups at 1280x720", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);

  const actualSections = await navigationSections(page, ".console-sidebar");
  expect(actualSections).toEqual(operatorNavigationGroups);
  const actualPaths = actualSections.flatMap((group) => group.paths);
  expect(actualPaths).toHaveLength(operatorPaths.length);
  expect(actualPaths).toEqual(operatorPaths);
});

test("operator navigation is one tab stop whose rows the arrow keys walk", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);

  const nav = page.locator(".console-sidebar .console-navigation");
  await page.locator(".console-brand a").focus();
  await page.keyboard.press("Tab");
  await expect(nav.locator('[data-navigation-item="/home"]')).toBeFocused();
  // Rows are reached by arrow, not by Tab: the fourth row is in the next section.
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("ArrowDown");
  const link = nav.locator('[data-navigation-item="/verification"]');
  await expect(link).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(new RegExp(`/verification\\?group=${selectedGroupID}$`));
  await expect(link).toHaveAttribute("aria-current", "page");
  // Tab leaves the tree in one step; Shift+Tab returns to the current row.
  await page.keyboard.press("Tab");
  await expect(nav.locator(":focus")).toHaveCount(0);
  await page.keyboard.press("Shift+Tab");
  await expect(link).toBeFocused();
});


test("manager navigation preserves the six groups while filtering only the instance-status destination", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "manager" });
  await openSpectrumRoute(page, "/groups");
  await expect(page.locator("[data-groups-source='api']")).toBeVisible();

  const actualSections = await navigationSections(page, ".console-sidebar");
  expect(actualSections).toEqual(managerNavigationGroups);
  const actualPaths = actualSections.flatMap((group) => group.paths);
  expect(actualPaths).toHaveLength(managerPaths.length);
  expect(actualPaths).toEqual(managerPaths);
  expect(actualPaths).not.toContain("/version");
});

test.describe("Spectrum shell geometry across changed routes", () => {
  for (const route of ["/home", "/queue"] as const) {
    for (const theme of ["light", "dark"] as const) {
      for (const width of [1280, 390, 320] as const) {
        test(`${route} ${theme} English layout uses the Spectrum scale at ${width}px`, async ({ page }) => {
          await page.addInitScript(() => {
            localStorage.setItem("verify-console-locale", "en");
          });
          await mockSpectrumTransport(page, { role: "operator" });
          await page.setViewportSize({ width, height: 720 });
          await openSpectrumRoute(page, route);
          if (route === "/home") await waitForHome(page);
          else await waitForQueue(page);
          const themeTrigger = page
            .locator('[data-utility-controls][data-variant="chrome"] [data-console-control]')
            .first()
            .locator("button")
            .first();
          await selectAppOption(themeTrigger, theme);
          await waitForFonts(page);
          const overflow = await horizontalGeometry(page);
          expect(overflow.document.scrollWidth).toBeLessThanOrEqual(overflow.document.clientWidth);
          expect(overflow.escapedElements).toEqual([]);
          expect(overflow.scopedScrollersOutsideViewport).toEqual([]);
          const geometry = await page.evaluate(() => {
            const content = document.querySelector<HTMLElement>(".console-content");
            const pageRoot = document.querySelector<HTMLElement>("[data-console-page]");
            const consoleHeader = document.querySelector<HTMLElement>(".console-header");
            return {
              contentPadding: content ? getComputedStyle(content).paddingInlineStart : null,
              contentPaddingBlock: content ? getComputedStyle(content).paddingBlockStart : null,
              headerGap: consoleHeader ? getComputedStyle(consoleHeader).gap : null,
              pageWidth: pageRoot?.getBoundingClientRect().width
            };
          });
          // The panel's padding steps down with the window: 32px, 20px under 800px tall, 16px on narrow screens.
          expect(geometry.contentPadding).toBe(width < 768 ? "16px" : "20px");
          expect(geometry.contentPaddingBlock).toBe(width < 768 ? "16px" : "20px");
          expect(geometry.headerGap).toBe("16px");
          if (width === 1280) {
            await page.setViewportSize({ width, height: 900 });
            expect(await page.locator(".console-content").evaluate((content) => getComputedStyle(content).paddingInlineStart)).toBe("32px");
            await page.setViewportSize({ width, height: 720 });
          }
          await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
          expect(geometry.pageWidth).toBeGreaterThan(0);
          const controls = await controlGeometry(page);
          expect(controls.length, `${route} must render measured controls`).toBeGreaterThan(0);
          for (const control of controls) {
            expect(control.innerHeight, JSON.stringify(control)).toBe(control.expectedHeight);
          }
        });
      }
    }
  }

  test.describe("touch scale", () => {
    test.use({ isMobile: true, hasTouch: true, viewport: { width: 390, height: 844 } });

    test("touch controls upgrade to XL and measure at the 1.25 Spectrum scale", async ({ page }) => {
      await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
      await mockSpectrumTransport(page, { role: "operator" });
      await openSpectrumRoute(page, "/queue");
      await waitForQueue(page);
      const grid = page.getByRole("grid");
      await grid.evaluate((element) => { element.scrollLeft = element.scrollWidth; });
      const action = grid.locator('[data-queue-action-id="release"]');
      const actionGeometry = await action.evaluate((element) => {
        const cell = element.closest('[role="gridcell"]')!;
        const inset = getComputedStyle(cell);
        return { button: element.getBoundingClientRect().width, cell: cell.getBoundingClientRect().width, insets: [inset.paddingLeft, inset.paddingRight] };
      });
      // What has to hold is that scrolling the grid to its end brings the action fully
      // into the scroller, not that a fraction of a pixel lands inside the viewport:
      // scrollLeft = scrollWidth settles on a fractional offset, and a viewport ratio of
      // exactly 1 turns that rounding into a failure. Measure the edge that matters.
      const clipping = await action.evaluate((element) => {
        const scroller = element.closest('[role="grid"]')!;
        const button = element.getBoundingClientRect();
        const bounds = scroller.getBoundingClientRect();
        return { overflowEnd: button.right - bounds.right, overflowStart: bounds.left - button.left };
      });
      expect(clipping.overflowEnd, JSON.stringify({ ...actionGeometry, ...clipping })).toBeLessThanOrEqual(1);
      expect(clipping.overflowStart, JSON.stringify({ ...actionGeometry, ...clipping })).toBeLessThanOrEqual(1);
      const controls = await controlGeometry(page);
      expect(controls.length).toBeGreaterThan(0);
      expect(controls.every((control) => control.size === "XL")).toBe(true);
      for (const control of controls) {
        expect(control.innerHeight, JSON.stringify(control)).toBe(60);
      }
    });
  });

  test("queue toolbar field and clear button have equal visible heights", async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await mockSpectrumTransport(page, { role: "operator" });
    await openSpectrumRoute(page, "/queue");
    await waitForQueue(page);

    const input = page.locator("[data-queue-filter] input");
    await expect(input).toBeVisible();
    await input.fill("@waiting");
    const clear = page.locator("[data-queue-filter-clear]");
    await expect(clear).toBeVisible();
    const heights = await page.evaluate(() => {
      const field = document.querySelector<HTMLElement>('[data-queue-filter] [role="presentation"]:has(> input)');
      const button = document.querySelector<HTMLElement>("[data-queue-filter-clear] button, [data-queue-filter-clear]");
      if (!field || !button) throw new Error("Queue filter controls are missing");
      return { input: field.getBoundingClientRect().height, button: button.getBoundingClientRect().height };
    });
    expect(heights.input).toBe(40);
    expect(heights.button).toBe(40);
    expect(heights.input).toBe(heights.button);
  });
});


// One test per theme pair rather than one test walking all four. Each pair loads three
// routes and measures every surface on them; doing all four in one test outgrew both the
// per-test budget and the runner, which closed the browser session mid-measurement.
for (const [preference, system] of [["light", "dark"], ["dark", "light"], ["system", "light"], ["system", "dark"]] as const) {
  test(`Spectrum layer surfaces cover the console shell and legacy cards with ${preference} on a ${system} system`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await mockSpectrumTransport(page, { role: "operator" });
    await page.emulateMedia({ colorScheme: system });
    await openSpectrumRoute(page, "/home");
    await waitForHome(page);
    const themeTrigger = page
      .locator('[data-utility-controls][data-variant="chrome"] [data-console-control]')
      .first()
      .locator("button")
      .first();
    await selectAppOption(themeTrigger, preference);
    await page.waitForFunction((expected) => (
      document.documentElement.dataset.themePreference === expected &&
      (expected === "system" ? !document.documentElement.hasAttribute("data-theme") : document.documentElement.dataset.theme === expected)
    ), preference);
    await page.evaluate(async () => {
      await Promise.all(document.getAnimations().map((animation) => animation.finished.catch(() => {})));
    });

    const nativeSurfaces = await page.evaluate(() => {
      const shell = document.querySelector<HTMLElement>("[data-app-shell]");
      const main = document.querySelector<HTMLElement>(".console-main");
      const panel = document.querySelector<HTMLElement>(".console-content");
      const sidebar = document.querySelector<HTMLElement>(".console-sidebar");
      const header = document.querySelector<HTMLElement>(".console-header");
      const brand = document.querySelector<HTMLElement>(".console-brand");
      // The home page's rows: a metric, an attention row and a configuration entry each
      // put their surface on the one element inside the link.
      const tiles = ["[data-home-metric] > *", "[data-home-attention] > *", "[data-home-entry] > *"]
        .map((selector) => document.querySelector<HTMLElement>(selector));
      const card = tiles[0];
      if (!shell || !main || !panel || !sidebar || !header || !brand || tiles.some((tile) => !tile)) {
        throw new Error("console layer surfaces are missing");
      }

      const isTransparent = (color: string): boolean => {
        if (color === "transparent") return true;
        const alpha = color.match(/\/\s*([0-9.]+)\s*\)$/)?.[1] ?? color.match(/^rgba\([^,]+,[^,]+,[^,]+,\s*([0-9.]+)\s*\)$/)?.[1];
        return alpha !== undefined && Number(alpha) === 0;
      };
      const effectiveBackground = (element: HTMLElement): string | null => {
        let current: HTMLElement | null = element;
        while (current) {
          const color = getComputedStyle(current).backgroundColor;
          if (!isTransparent(color)) return color;
          current = current.parentElement;
        }
        return null;
      };
      const channels = (color: string): readonly number[] | null => {
        const values = color.match(/[0-9.]+/g)?.slice(0, 3).map(Number) ?? [];
        return values.length === 3 && values.every((value) => Number.isFinite(value)) ? values : null;
      };
      const luminance = (color: string | null): number | null => {
        const values = color === null ? null : channels(color);
        if (!values) return null;
        return values.reduce((sum, value, index) => {
          const channel = value / 255;
          const linear = channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
          return sum + linear * [0.2126, 0.7152, 0.0722][index];
        }, 0);
      };
      const sidebarBounds = sidebar.getBoundingClientRect();
      const panelBounds = panel.getBoundingClientRect();
      const headerBounds = header.getBoundingClientRect();
      const outerBackground = effectiveBackground(shell);
      const panelBackground = effectiveBackground(panel);

      return {
        outerBackground,
        panelBackground,
        outerLuminance: luminance(outerBackground),
        panelLuminance: luminance(panelBackground),
        card: getComputedStyle(card!).backgroundColor,
        cardLuminance: luminance(getComputedStyle(card!).backgroundColor),
        tileBackgrounds: tiles.map((tile) => getComputedStyle(tile!).backgroundColor),
        tileRadii: tiles.map((tile) => getComputedStyle(tile!).borderTopLeftRadius),
        tileBorders: tiles.map((tile) => getComputedStyle(tile!).borderTopWidth),
        radii: [
          getComputedStyle(panel).borderTopLeftRadius,
          getComputedStyle(panel).borderTopRightRadius,
          getComputedStyle(panel).borderBottomRightRadius,
          getComputedStyle(panel).borderBottomLeftRadius
        ],
        panelWidth: panelBounds.width,
        panelHeight: panelBounds.height,
        leftGap: panelBounds.left - sidebarBounds.right,
        sidebarLeftGap: sidebarBounds.left,
        topGap: panelBounds.top - headerBounds.bottom,
        rightGap: window.innerWidth - panelBounds.right,
        sidebarBackground: effectiveBackground(sidebar),
        headerBackground: effectiveBackground(header),
        sidebarRightBorder: getComputedStyle(sidebar).borderInlineEndWidth,
        sidebarPhysicalRightBorder: getComputedStyle(sidebar).borderRightWidth,
        headerBottomBorder: getComputedStyle(header).borderBottomWidth,
        brandBottomBorder: getComputedStyle(brand).borderBottomWidth
      };
    });
    expect(nativeSurfaces.outerBackground).not.toBeNull();
    expect(nativeSurfaces.panelBackground).not.toBeNull();
    expect(nativeSurfaces.sidebarBackground).toBe(nativeSurfaces.outerBackground);
    expect(nativeSurfaces.headerBackground).toBe(nativeSurfaces.outerBackground);
    expect(nativeSurfaces.panelBackground).not.toBe(nativeSurfaces.outerBackground);
    expect(nativeSurfaces.panelLuminance).toBeGreaterThan(nativeSurfaces.outerLuminance!);
    for (const radius of nativeSurfaces.radii.slice(0, 2)) {
      expect(parseFloat(radius)).toBeGreaterThan(0);
    }
    for (const radius of nativeSurfaces.radii.slice(2)) {
      expect(parseFloat(radius)).toBe(0);
    }
    expect(nativeSurfaces.panelWidth).toBeGreaterThan(0);
    expect(nativeSurfaces.panelHeight).toBeGreaterThan(0);
    expect(nativeSurfaces.leftGap).toBe(0);
    expect(nativeSurfaces.sidebarLeftGap).toBeGreaterThan(0);
    expect(nativeSurfaces.topGap).toBe(0);
    expect(nativeSurfaces.rightGap).toBeGreaterThan(0);
    expect(nativeSurfaces.sidebarRightBorder).toBe("0px");
    expect(nativeSurfaces.sidebarPhysicalRightBorder).toBe("0px");
    expect(nativeSurfaces.headerBottomBorder).toBe("0px");
    expect(nativeSurfaces.brandBottomBorder).toBe("0px");
    // One tint level inside the panel: every home row is the same fill, one step back
    // towards the page ground, rounded, and separated by colour rather than a border.
    expect(new Set(nativeSurfaces.tileBackgrounds).size).toBe(1);
    expect(nativeSurfaces.card).not.toBe(nativeSurfaces.panelBackground);
    expect(nativeSurfaces.card).toBe(nativeSurfaces.outerBackground);
    expect(new Set(nativeSurfaces.tileRadii).size).toBe(1);
    expect(parseFloat(nativeSurfaces.tileRadii[0]!)).toBeGreaterThan(0);
    expect(nativeSurfaces.tileBorders).toEqual(["0px", "0px", "0px"]);

    await page.setViewportSize({ width: 1280, height: 600 });
    const panelScroll = await page.evaluate(() => {
      const panel = document.querySelector<HTMLElement>(".console-content");
      if (!panel) throw new Error("console content panel is missing");
      panel.scrollTop = panel.scrollHeight;
      return {
        overflowY: getComputedStyle(panel).overflowY,
        scrollHeight: panel.scrollHeight,
        clientHeight: panel.clientHeight,
        scrollTop: panel.scrollTop,
        documentHeight: Math.max(document.documentElement.scrollHeight, document.body.scrollHeight),
        viewportHeight: window.innerHeight
      };
    });
    expect(["auto", "scroll"]).toContain(panelScroll.overflowY);
    expect(panelScroll.scrollHeight).toBeGreaterThan(panelScroll.clientHeight);
    expect(panelScroll.scrollTop).toBeGreaterThan(0);
    expect(panelScroll.documentHeight).toBeLessThanOrEqual(panelScroll.viewportHeight + 1);
    await page.setViewportSize({ width: 1280, height: 720 });

    await openSpectrumRoute(page, "/groups");
    await expect(page.locator("[data-groups-page]")).toBeVisible();
    await expect(page.locator("[data-group-row], [data-group-state]").first()).toBeVisible();
    // Legacy cards take the same one step back as the home rows, and no second edge.
    const legacySurface = await page.locator('[data-slot="card"]').first().evaluate((element) => ({
      background: getComputedStyle(element).backgroundColor,
      border: getComputedStyle(element).borderTopWidth,
      shadow: getComputedStyle(element).boxShadow
    }));
    expect(legacySurface.background).toBe(nativeSurfaces.card);
    expect(legacySurface.border).toBe("0px");
    expect(legacySurface.shadow).toBe("none");
    await openSpectrumRoute(page, "/queue");
    await expect(page.locator("[data-queue-toolbar]")).toBeVisible();
    expect(await page.locator("[data-queue-toolbar]").evaluate((element) => getComputedStyle(element).backgroundColor)).toBe(nativeSurfaces.card);
  });
}


test("mobile navigation uses a portalled dialog, restores focus on Escape, and closes after selecting a link", async ({
  page
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);

  const trigger = page.locator("[data-mobile-navigation] button");
  await expect(trigger).toHaveAttribute("aria-haspopup", "dialog");
  await trigger.click();
  const panel = page.locator(".console-mobile-panel");
  await expect(panel).toBeVisible();
  expect(await panel.evaluate((element) => !element.closest("[data-mobile-navigation]"))).toBe(true);
  expect(await navigationSections(page, ".console-mobile-panel")).toEqual(operatorNavigationGroups);

  await page.keyboard.press("Escape");
  await expect(panel).toBeHidden();
  await expect.poll(() => trigger.evaluate((element) => document.activeElement === element)).toBe(true);

  await trigger.click();
  const destination = panel.locator('[data-navigation-item="/preferences"]');
  await expect(destination).toBeVisible();
  await destination.click();
  await expect(page).toHaveURL((url) =>
    url.pathname === "/preferences" && url.searchParams.get("group") === selectedGroupID
  );
  await expect(panel).toBeHidden();
});

test("home chart combines seven-day counts and rates with dual axes, labels, legend, and hover readings", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator", trend: "full" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);
  await waitForFonts(page);

  const chart = page.getByTestId("home-combined-chart");
  await expect(chart).toHaveCount(1);
  await expect(chart.locator("svg")).toBeVisible();
  const trendBox = await page.locator('[data-home-section="trend"] [data-home-trend-chart]').boundingBox();
  expect(trendBox?.height).toBeGreaterThan(0);
  expect(trendBox?.height).toBeLessThanOrEqual(420);
  const chartGeometry = await chart.evaluate((element) => {
    const svg = element.querySelector("svg");
    const plot = element.closest<HTMLElement>("[data-home-trend-scroll]")?.firstElementChild;
    const scroll = element.closest<HTMLElement>("[data-home-trend-scroll]");
    if (!svg || !(plot instanceof HTMLElement) || !scroll) throw new Error("Home chart geometry is missing");
    const bounds = svg.getBoundingClientRect();
    return {
      svgHeight: bounds.height,
      plotHeight: plot.getBoundingClientRect().height,
      scrollHeight: scroll.scrollHeight,
      scrollClientHeight: scroll.clientHeight,
      clippedLabels: Array.from(svg.querySelectorAll("text")).filter((label) => {
        const box = label.getBoundingClientRect();
        return box.left < bounds.left || box.right > bounds.right || box.top < bounds.top || box.bottom > bounds.bottom;
      }).map((label) => label.textContent)
    };
  });
  expect(chartGeometry.svgHeight).toBeGreaterThan(0);
  expect(chartGeometry.plotHeight).toBeGreaterThan(0);
  expect(chartGeometry.svgHeight).toBeLessThanOrEqual(chartGeometry.plotHeight + 1);
  expect(chartGeometry.scrollHeight).toBeLessThanOrEqual(chartGeometry.scrollClientHeight + 1);
  expect(chartGeometry.clippedLabels).toEqual([]);

  // The bars carry their counts as the library's direct labels. The line carries no
  // per-point label: the chart library places those through Vega's label transform,
  // which drops any it cannot fit, so the rates read from the hover and the reading.
  const labels = chart.locator(".role-mark.requestsDirectLabel0 text");
  await expect(labels).toHaveText(chartDays.map((day) => String(day.challenges)));
  await expect(chart.locator(".mark-text.role-mark text").filter({ hasText: "%" })).toHaveCount(0);

  const yAxes = chart.locator('[aria-label^="Y-axis"]');
  await expect(yAxes).toHaveCount(2);
  await expect(yAxes.locator(".role-axis-label text").first()).toBeVisible();
  const axisLabels = await yAxes.locator(".role-axis-label text").allTextContents();
  expect(axisLabels.some((label) => /^\d+$/.test(label))).toBe(true);
  expect(axisLabels).toContain("0%");
  expect(axisLabels).toContain("100%");
  const chartBox = await chart.boundingBox();
  if (!chartBox) throw new Error("Home chart has no bounds");
  const axisSides = await yAxes.evaluateAll((axes) => axes.map((axis) => {
    const label = axis.querySelector(".role-axis-label text");
    return label?.getBoundingClientRect().left ?? NaN;
  }));
  expect(axisSides.some((left) => left < chartBox.x + chartBox.width / 2)).toBe(true);
  expect(axisSides.some((left) => left > chartBox.x + chartBox.width / 2)).toBe(true);
  await expect(chart.locator('[aria-label^="X-axis"] .role-axis-label text')).toHaveText(
    chartDays.map((day) => new Intl.DateTimeFormat("en-US", {
      timeZone: "UTC", month: "numeric", day: "numeric"
    }).format(new Date(`${day.date}T00:00:00Z`)))
  );
  await expect(chart.locator('[aria-label^="X-axis"] .role-axis-label text')).toHaveCount(7);
  const legend = page.locator("[data-home-trend-legend]");
  await expect(legend).toContainText(/Requests.*left axis/);
  await expect(legend).toContainText(/Pass rate.*right axis/);
  await expect(legend.locator("[data-icon-name]")).toHaveCount(2);

  await expect(page.locator('[data-home-metric="challenges"]')).toContainText("70");
  await expect(page.locator('[data-home-metric="pass-rate"]')).toContainText("58.6%");
  await expect(page.locator('[data-home-metric="waiting"]')).toContainText("2");
  await expect(page.locator('[data-home-metric="banned"]')).toContainText("4");

  // The line's transparent hover region lies over the bars, so the pointer lands on it
  // rather than on the bar; both carry the same reading for the day.
  await chart.locator(".mark-rect.role-mark.requests path").first().hover({ force: true });
  await expect(page.locator("#vg-tooltip-element")).toHaveText("Aug 26: 8 challenges, 50% pass rate");
  await chart.locator(".mark-symbol.role-mark path").last().hover({ force: true });
  await expect(page.locator("#vg-tooltip-element")).toHaveText("Sep 1: 8 challenges, 75% pass rate");
});


test("zero-day combined chart data keeps exact zero readings and never emits NaN", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page, { role: "operator", trend: "zero" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);

  const chart = page.getByTestId("home-combined-chart");
  await expect(chart.locator(".role-mark.requestsDirectLabel0 text")).toHaveText(chartDays.map(() => "0"));
  expect(await chart.innerHTML()).not.toContain("NaN");
  await expect(page.locator("[data-home-chart-reading]")).toHaveText("Aug 26: 0 challenges, 0% pass rate");
  await expect(page.locator('[data-home-metric="challenges"]')).toContainText("0");
  await expect(page.locator('[data-home-metric="pass-rate"]')).toContainText("0%");
});

test("pointer press and reduced motion remain observable on controls", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/queue");
  await waitForQueue(page);
  const action = page.locator('[data-queue-action-id="release"]').first();
  await expect(action).toBeVisible();
  const actionBox = await action.boundingBox();
  if (!actionBox) throw new Error("Queue action has no hit area");
  await page.mouse.move(actionBox.x + actionBox.width / 2, actionBox.y + actionBox.height / 2);
  const idleAction = await action.evaluate((element) => getComputedStyle(element).transform);
  await page.mouse.down();
  await expect.poll(() => action.evaluate((element) => getComputedStyle(element).transform)).not.toBe(idleAction);
  await page.mouse.up();

  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.reload();
  await waitForQueue(page);
  const reducedBox = await action.boundingBox();
  if (!reducedBox) throw new Error("Reduced-motion action has no hit area");
  await page.mouse.move(reducedBox.x + reducedBox.width / 2, reducedBox.y + reducedBox.height / 2);
  await page.mouse.down();
  await page.evaluate(() => new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  ));
  const reduced = await page.locator('[data-queue-action-id="release"]').first().evaluate((element) => {
    const inner = element.matches("button")
      ? element
      : element.querySelector<HTMLElement>("button, [role='button']") ?? element;
    return { transform: getComputedStyle(inner).transform };
  });
  await page.mouse.up();
  expect(reduced.transform).toBe("none");
});
