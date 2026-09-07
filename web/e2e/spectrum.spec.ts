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
              pageGap: pageRoot ? getComputedStyle(pageRoot).rowGap : null,
              headerGap: consoleHeader ? getComputedStyle(consoleHeader).gap : null,
              pageWidth: pageRoot?.getBoundingClientRect().width
            };
          });
          expect(geometry.contentPadding).toBe(width < 768 ? "16px" : "32px");
          expect(geometry.contentPaddingBlock).toBe(width < 768 ? "16px" : "32px");
          expect(geometry.contentOverflowX).toBe("visible");
          expect(geometry.headerGap).toBe("16px");
          await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
          expect(geometry.pageGap).toBe("24px");
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

test("home chart exposes exact seven-day values, separate count/rate scales, and readable hover and focus states", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator", trend: "full" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);

  await expect(page.locator("[data-home-trend-chart]")).toBeVisible();
  const points = await page.locator('[data-home-chart-point][data-home-chart-series="count"]').evaluateAll((elements) =>
    elements.map((element) => ({
      date: element.getAttribute("data-home-chart-date"),
      value: Number(element.getAttribute("data-home-chart-value")),
      label: element.querySelector("[data-home-chart-count-label]")?.textContent?.trim()
    }))
  );
  expect(points).toEqual(
    chartDays.map(({ date, challenges }) => ({ date, value: challenges, label: String(challenges) }))
  );
  const rates = await page.locator('[data-home-chart-point][data-home-chart-series="rate"]').evaluateAll((elements) =>
    elements.map((element) => ({
      date: element.getAttribute("data-home-chart-date"),
      value: Number(element.getAttribute("data-home-chart-pass-rate")),
      label: element.querySelector("[data-home-chart-rate-label]")?.textContent?.trim()
    }))
  );
  expect(rates.map(({ date, value }) => ({ date, value }))).toEqual(
    chartDays.map(({ date, passRate }) => ({ date, value: passRate }))
  );
  expect(rates.every(({ label }) => label?.includes("%"))).toBe(true);

  const ticks = await page.locator("[data-home-chart-tick]").evaluateAll((elements) =>
    elements.map((element) => ({
      scale: element.getAttribute("data-home-chart-scale"),
      value: Number(element.getAttribute("data-home-chart-tick"))
    }))
  );
  expect(ticks.filter((tick) => tick.scale === "count").map((tick) => tick.value)).toEqual([0, 4, 8, 12, 15]);
  expect(ticks.filter((tick) => tick.scale === "rate").map((tick) => tick.value)).toEqual([0, 0.25, 0.5, 0.75, 1]);
  expect(new Set(ticks.map((tick) => tick.scale))).toEqual(new Set(["count", "rate"]));
  for (const label of await page.locator(
    "[data-home-chart-axis], [data-home-chart-count-label], [data-home-chart-rate-label], [data-home-chart-date-label]"
  ).all()) {
    await expect(label).toBeVisible();
  }

  await expect(page.locator('[data-home-metric="challenges"]')).toContainText("70");
  await expect(page.locator('[data-home-metric="pass-rate"]')).toContainText("58.6%");
  await expect(page.locator('[data-home-metric="waiting"]')).toContainText("2");
  await expect(page.locator('[data-home-metric="banned"]')).toContainText("4");

  const firstCountPoint = page.locator('[data-home-chart-point][data-home-chart-series="count"]').first();
  const restOpacity = await firstCountPoint.locator("[data-home-chart-bar]").evaluate(
    (element) => getComputedStyle(element).opacity
  );
  await firstCountPoint.locator("[data-home-chart-bar]").hover();
  await expect.poll(() => firstCountPoint.locator("[data-home-chart-bar]").evaluate(
    (element) => getComputedStyle(element).opacity
  )).not.toBe(restOpacity);
  await page.keyboard.press("Tab");
  await firstCountPoint.focus();
  await expect.poll(() => firstCountPoint.evaluate((element) => document.activeElement === element)).toBe(true);
  await expect(firstCountPoint).toHaveAttribute("aria-label", /8/);
  await expect(firstCountPoint).toHaveAttribute("aria-label", /50%/);
  const focusStyle = await firstCountPoint.evaluate((element) => {
    const style = getComputedStyle(element);
    return { outlineWidth: style.outlineWidth, outlineStyle: style.outlineStyle };
  });
  expect(focusStyle.outlineStyle).not.toBe("none");
  expect(Number.parseFloat(focusStyle.outlineWidth)).toBeGreaterThanOrEqual(2);
});

test("zero-day chart data keeps exact zero readings and never emits NaN", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page, { role: "operator", trend: "zero" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);

  const chart = page.locator("[data-home-trend-chart]");
  await expect(chart).toBeVisible();
  const chartText = await chart.textContent();
  expect(chartText).not.toContain("NaN");
  const countValues = await chart.locator('[data-home-chart-point][data-home-chart-series="count"]').evaluateAll(
    (elements) => elements.map((element) => Number(element.getAttribute("data-home-chart-value")))
  );
  const rateValues = await chart.locator('[data-home-chart-point][data-home-chart-series="rate"]').evaluateAll(
    (elements) => elements.map((element) => Number(element.getAttribute("data-home-chart-pass-rate")))
  );
  expect(countValues).toEqual(chartDays.map(() => 0));
  expect(rateValues).toEqual(chartDays.map(() => 0));
  const zeroCountLabels = await chart.locator("[data-home-chart-count-label]").allTextContents();
  const zeroRateLabels = await chart.locator("[data-home-chart-rate-label]").allTextContents();
  expect(zeroCountLabels).toEqual(chartDays.map(() => "0"));
  expect(zeroRateLabels).toEqual(chartDays.map(() => "0%"));
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
