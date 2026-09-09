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

function routeURL(path: string): RegExp {
  return new RegExp(`${path.replaceAll("/", "\\/")}\\?group=${selectedGroupID}$`);
}

async function waitForHome(page: Page): Promise<void> {
  await expect(page.locator("[data-home-page]")).toHaveAttribute("data-home-state", "loaded");
}

async function waitForQueue(page: Page): Promise<void> {
  await expect(page.locator("[data-queue-page]")).toHaveAttribute("data-queue-state", "populated");
}

async function expandNavigationGroup(page: Page, root: string, id: string) {
  const group = page.locator(`${root} [data-navigation-group="${id}"]`);
  if (await group.getAttribute("aria-expanded") === "false") await group.click();
  await expect(group).toHaveAttribute("aria-expanded", "true");
}

async function navigationSections(page: Page, root: string) {
  const groups = page.locator(`${root} [data-navigation-group]`);
  const sections = [];
  for (let index = 0; index < await groups.count(); index++) {
    const id = (await groups.nth(index).getAttribute("data-navigation-group"))!;
    await expandNavigationGroup(page, root, id);
    const paths = await page.locator(`${root} nav a[href]`).evaluateAll((links) =>
      links.map((link) => new URL((link as HTMLAnchorElement).href).pathname)
    );
    sections.push({ id, paths });
  }
  return sections;
}

async function focusNavigationDestination(page: Page, path: string): Promise<void> {
  await page.locator(".console-brand a").focus();
  await page.keyboard.press("Tab");
  await expect(page.locator('.console-sidebar nav a[href^="/home"]')).toBeFocused();
  await page.keyboard.press("ArrowLeft");
  for (const section of operatorNavigationGroups) {
    const group = page.locator(`.console-sidebar [data-navigation-group="${section.id}"]`);
    await expect.poll(() => group.evaluate((element) => element.contains(document.activeElement)), {
      message: `Keyboard navigation reaches the ${section.id} group`
    }).toBe(true);
    if (section.paths.some((destination) => destination === path)) {
      if (await group.getAttribute("aria-expanded") === "false") await page.keyboard.press("ArrowRight");
      await expect(group).toHaveAttribute("aria-expanded", "true");
      for (const destination of section.paths) {
        await page.keyboard.press("ArrowDown");
        await expect(page.locator(`.console-sidebar nav a[href^="${destination}"]`)).toBeFocused();
        if (destination === path) return;
      }
    } else {
      if (await group.getAttribute("aria-expanded") === "true") await page.keyboard.press("ArrowLeft");
      await expect(group).toHaveAttribute("aria-expanded", "false");
      await page.keyboard.press("ArrowDown");
    }
  }
  throw new Error(`Keyboard navigation did not reach ${path}`);
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

for (const path of operatorPaths) {
  test(`operator keyboard navigation reaches ${path} at 1280x720`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 720 });
    await mockSpectrumTransport(page, { role: "operator" });
    await openSpectrumRoute(page, "/home");
    await waitForHome(page);
    await focusNavigationDestination(page, path);
    const link = page.locator(`.console-sidebar nav a[href^="${path}"]`);
    await expect(link, `${path} must have one declared destination`).toHaveCount(1);
    await link.scrollIntoViewIfNeeded();
    const viewport = await link.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const target = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
      return {
        left: rect.left,
        top: rect.top,
        right: rect.right,
        bottom: rect.bottom,
        width: rect.width,
        height: rect.height,
        hit: target === element || target instanceof Element && element.contains(target)
      };
    });
    expect(viewport.width, `${path} must have a hit area`).toBeGreaterThan(0);
    expect(viewport.height, `${path} must have a hit area`).toBeGreaterThan(0);
    expect(viewport.left, `${path} must be inside the viewport`).toBeGreaterThanOrEqual(0);
    expect(viewport.top, `${path} must be inside the viewport`).toBeGreaterThanOrEqual(0);
    expect(viewport.right, `${path} must be inside the viewport`).toBeLessThanOrEqual(1280);
    expect(viewport.bottom, `${path} must be inside the viewport`).toBeLessThanOrEqual(720);
    expect(viewport.hit, `${path} must be hit-testable after scrolling`).toBe(true);

    await expect(link).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(routeURL(path));
  });
}

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
              contentOverflowX: content ? getComputedStyle(content).overflowX : null,
              headerGap: consoleHeader ? getComputedStyle(consoleHeader).gap : null,
              pageWidth: pageRoot?.getBoundingClientRect().width
            };
          });
          expect(geometry.contentPadding).toBe(width < 768 ? "16px" : "32px");
          expect(geometry.contentPaddingBlock).toBe(width < 768 ? "16px" : "32px");
          expect(geometry.contentOverflowX).toBe("visible");
          expect(geometry.headerGap).toBe("16px");
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
      await expect(action, JSON.stringify(actionGeometry)).toBeInViewport({ ratio: 1 });
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


test("Spectrum layer surfaces cover the console shell and legacy cards in every theme", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });

  for (const [preference, system] of [["light", "dark"], ["dark", "light"], ["system", "light"], ["system", "dark"]] as const) {
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
      const provider = shell?.parentElement;
      const main = document.querySelector<HTMLElement>(".console-main");
      const sidebar = document.querySelector<HTMLElement>(".console-sidebar");
      const header = document.querySelector<HTMLElement>(".console-header");
      const card = document.querySelector<HTMLElement>("[data-console-card]");
      if (!provider || !main || !sidebar || !header || !card) {
        throw new Error("console layer surfaces are missing");
      }
      return {
        provider: getComputedStyle(provider).backgroundColor,
        content: getComputedStyle(main).backgroundColor,
        sidebar: getComputedStyle(sidebar).backgroundColor,
        header: getComputedStyle(header).backgroundColor,
        card: getComputedStyle(card).backgroundColor,
        cardShadow: getComputedStyle(card).boxShadow
      };
    });
    const dark = preference === "dark" || (preference === "system" && system === "dark");
    const expected = dark
      ? { provider: "rgb(17, 17, 17)", content: "rgb(27, 27, 27)", card: "rgb(34, 34, 34)" }
      : { provider: "rgb(255, 255, 255)", content: "rgb(248, 248, 248)", card: "rgb(255, 255, 255)" };
    expect(nativeSurfaces.provider).toBe(nativeSurfaces.sidebar);
    expect(nativeSurfaces.provider).toBe(nativeSurfaces.header);
    expect(nativeSurfaces.provider).toBe(expected.provider);
    expect(nativeSurfaces.content).toBe(expected.content);
    expect(nativeSurfaces.card).toBe(expected.card);
    const contentBrightness = nativeSurfaces.content.match(/\d+/g)!.slice(0, 3).reduce((sum, channel) => sum + Number(channel), 0);
    const cardBrightness = nativeSurfaces.card.match(/\d+/g)!.slice(0, 3).reduce((sum, channel) => sum + Number(channel), 0);
    expect(cardBrightness).toBeGreaterThan(contentBrightness);
    expect(nativeSurfaces.cardShadow).not.toBe("none");

    await openSpectrumRoute(page, "/groups");
    await expect(page.locator("[data-groups-page]")).toBeVisible();
    await expect(page.locator("[data-group-row], [data-group-state]").first()).toBeVisible();
    const legacySurface = await page.locator('[data-slot="card"]').first().evaluate((element) => ({
      background: getComputedStyle(element).backgroundColor
    }));
    expect(legacySurface.background).toBe(nativeSurfaces.card);
    await openSpectrumRoute(page, "/queue");
    await expect(page.locator("[data-queue-toolbar]")).toBeVisible();
    expect(await page.locator("[data-queue-toolbar]").evaluate((element) => getComputedStyle(element).backgroundColor)).toBe(nativeSurfaces.card);
  }
});


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
  await expandNavigationGroup(page, ".console-mobile-panel", "console");
  const destination = panel.locator('a[href^="/preferences"]');
  await expect(destination).toBeVisible();
  await destination.click();
  await expect(page).toHaveURL(routeURL("/preferences"));
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

  const labels = chart.locator(".mark-text.role-mark text");
  await expect(labels).toHaveText([
    ...chartDays.map((day) => String(day.challenges)),
    "50%", "58%", "43%", "67%", "56%", "64%", "75%"
  ]);

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
  await expect(chart.locator(".role-legend")).toContainText(/Requests.*left axis/);
  await expect(chart.locator(".role-legend")).toContainText(/Pass rate.*right axis/);
  await expect(chart.locator(".role-legend-symbol")).toHaveCount(2);

  await expect(page.locator('[data-home-metric="challenges"]')).toContainText("70");
  await expect(page.locator('[data-home-metric="pass-rate"]')).toContainText("58.6%");
  await expect(page.locator('[data-home-metric="waiting"]')).toContainText("2");
  await expect(page.locator('[data-home-metric="banned"]')).toContainText("4");

  await chart.locator(".mark-rect.role-mark path").first().hover();
  await expect(page.locator("#vg-tooltip-element")).toHaveText("Aug 26: 8 challenges, 50% pass rate");
  await chart.locator(".mark-symbol.role-mark path").last().hover();
  await expect(page.locator("#vg-tooltip-element")).toHaveText("Sep 1: 8 challenges, 75% pass rate");
});


test("zero-day combined chart data keeps exact zero readings and never emits NaN", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page, { role: "operator", trend: "zero" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);

  const chart = page.getByTestId("home-combined-chart");
  await expect(chart.locator(".mark-text.role-mark text")).toHaveText([
    ...chartDays.map(() => "0"),
    ...chartDays.map(() => "0%")
  ]);
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
