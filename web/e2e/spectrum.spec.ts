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

async function navigationSections(page: Page, root: string) {
  return page.locator(`${root} [data-navigation-group]`).evaluateAll((elements) =>
    elements.map((element) => ({
      id: element.getAttribute("data-navigation-group"),
      paths: [...element.querySelectorAll<HTMLAnchorElement>("a[href]")].map(
        (link) => new URL(link.href).pathname
      )
    }))
  );
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
      const rootStyle = getComputedStyle(root);
      const innerStyle = getComputedStyle(face);
      return {
        size: root.getAttribute("data-control-size"),
        rootRadius: rootStyle.borderRadius,
        innerRadius: innerStyle.borderRadius,
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

function colourHue(value: string): number | null {
  const channels = value.match(/\d+(?:\.\d+)?/g)?.map(Number);
  if (!channels || channels.length < 3) return null;
  const [red, green, blue] = channels;
  const maximum = Math.max(red, green, blue);
  const minimum = Math.min(red, green, blue);
  if (maximum === minimum) return null;
  const delta = maximum - minimum;
  let hue = 0;
  if (maximum === red) hue = ((green - blue) / delta) % 6;
  else if (maximum === green) hue = (blue - red) / delta + 2;
  else hue = (red - green) / delta + 4;
  hue *= 60;
  return hue < 0 ? hue + 360 : hue;
}

test("operator navigation exposes all 15 destinations that scroll into the 1280x720 viewport", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await waitForHome(page);

  const actualSections = await navigationSections(page, ".console-sidebar");
  expect(actualSections).toEqual(operatorNavigationGroups);
  const actualPaths = actualSections.flatMap((group) => group.paths);
  expect(actualPaths).toHaveLength(operatorPaths.length);
  expect(actualPaths).toEqual(operatorPaths);

  for (const path of operatorPaths) {
    const link = page.locator(`.console-sidebar .console-nav-link[href^="${path}"]`);
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

    await link.focus();
    await expect.poll(() => link.evaluate((element) => document.activeElement === element)).toBe(true);
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(routeURL(path));
    await openSpectrumRoute(page, "/home");
    await waitForHome(page);
  }
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
  test("light and dark English layouts retain page, section, and card spacing at desktop and narrow widths", async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("verify-console-locale", "en");
    });
    await mockSpectrumTransport(page, { role: "operator" });

    for (const route of ["/home", "/queue"] as const) {
      for (const theme of ["light", "dark"] as const) {
        for (const width of [1280, 390, 320] as const) {
          await test.step(`${route} ${theme} ${width}px`, async () => {
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
              const sections = [...document.querySelectorAll<HTMLElement>("[data-home-section]")];
              const cards = [...document.querySelectorAll<HTMLElement>("[data-console-card]")].filter((card) =>
                card.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })
              );
              const visibleBorder = (element: HTMLElement) => {
                const style = getComputedStyle(element);
                return {
                  width: style.borderBlockStartWidth,
                  color: style.borderBlockStartColor,
                  radius: style.borderRadius,
                  padding: style.padding
                };
              };
              return {
                contentPadding: content ? getComputedStyle(content).paddingInlineStart : null,
                contentPaddingBlock: content ? getComputedStyle(content).paddingBlockStart : null,
                contentOverflowX: content ? getComputedStyle(content).overflowX : null,
                pageGap: pageRoot ? getComputedStyle(pageRoot).rowGap : null,
                headerGap: consoleHeader ? getComputedStyle(consoleHeader).gap : null,
                sections: sections.map((section) => {
                  const style = getComputedStyle(section);
                  return {
                    card: section.hasAttribute("data-console-card"),
                    gap: style.rowGap,
                    border: visibleBorder(section),
                    paddingBlockStart: style.paddingBlockStart,
                    padding: style.padding
                  };
                }),
                cards: cards.map(visibleBorder)
              };
            });

            expect(geometry.contentPadding).toBe(width < 768 ? "16px" : "32px");
            expect(geometry.contentPaddingBlock).toBe(width < 768 ? "16px" : "32px");
            expect(geometry.contentOverflowX).toBe("visible");
            expect(geometry.headerGap).toBe("16px");
            await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
            expect(geometry.pageGap).toBe("24px");
            if (route === "/home") {
              const badges = await page.locator("[data-home-context-badges]").evaluate((element) => ({
                scrollWidth: element.scrollWidth,
                clientWidth: element.clientWidth
              }));
              expect(badges.scrollWidth).toBeLessThanOrEqual(badges.clientWidth + 1);
              const nonCardSections = geometry.sections.filter((section) => !section.card);
              const cardSections = geometry.sections.filter((section) => section.card);
              expect(nonCardSections.length).toBeGreaterThan(0);
              expect(new Set(nonCardSections.map((section) => section.gap))).toEqual(new Set(["16px"]));
              expect(new Set(nonCardSections.map((section) => section.paddingBlockStart))).toEqual(
                new Set(["24px"])
              );
              for (const section of nonCardSections) {
                expect(section.border.width).toBe("1px");
                expect(section.border.color).not.toBe("rgba(0, 0, 0, 0)");
              }
              expect(cardSections.length).toBeGreaterThan(0);
              expect(new Set(cardSections.map((section) => section.padding))).toEqual(new Set(["16px"]));
            }
            expect(geometry.cards.length).toBeGreaterThan(0);
            for (const card of geometry.cards) {
              expect(card.width).toBe("1px");
              expect(card.color).not.toBe("rgba(0, 0, 0, 0)");
              expect(card.radius).toBe("12px");
              expect(card.padding).toBe("16px");
            }

            const controls = await controlGeometry(page);
            expect(controls.length, `${route} must render measured controls`).toBeGreaterThan(0);
            for (const control of controls) {
              expect(control.rootRadius, JSON.stringify(control)).toBe("8px");
              expect(control.innerRadius, JSON.stringify(control)).toBe("8px");
              expect(control.innerHeight, JSON.stringify(control)).toBe(control.expectedHeight);
            }
          });
        }
      }
    }
  });

  test.describe("touch scale", () => {
    test.use({ isMobile: true, hasTouch: true, viewport: { width: 390, height: 844 } });

    test("touch controls upgrade to XL and measure at the 1.25 Spectrum scale", async ({ page }) => {
      await mockSpectrumTransport(page, { role: "operator" });
      await openSpectrumRoute(page, "/queue");
      await waitForQueue(page);
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
  const destination = panel.locator('.console-nav-link[href^="/preferences"]');
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

test("pointer press, visible focus, accent color, and reduced motion remain observable on controls", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/queue");
  await waitForQueue(page);
  const routeMotion = await page.locator(".console-inner").evaluate((element) => {
    const style = getComputedStyle(element);
    return { name: style.animationName, duration: style.animationDuration };
  });
  expect(routeMotion.name).not.toBe("none");
  expect(Number.parseFloat(routeMotion.duration)).toBeGreaterThan(0);

  const activeLink = page.locator('.console-nav-link[aria-current="page"]');
  await activeLink.focus();
  const activeStyle = await activeLink.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      outlineWidth: style.outlineWidth,
      outlineStyle: style.outlineStyle,
      color: style.color
    };
  });
  expect(activeStyle.outlineStyle).not.toBe("none");
  expect(Number.parseFloat(activeStyle.outlineWidth)).toBeGreaterThanOrEqual(2);
  const hue = colourHue(activeStyle.color);
  expect(hue, `accent must resolve to a blue hue, got ${activeStyle.color}`).not.toBeNull();
  expect(hue as number).toBeGreaterThan(190);
  expect(hue as number).toBeLessThan(250);

  const idleLink = page.locator('.console-nav-link:not([aria-current="page"])').first();
  const idleStyle = await idleLink.evaluate((element) => {
    const style = getComputedStyle(element);
    return { background: style.backgroundColor, color: style.color };
  });
  await idleLink.hover();
  await expect.poll(() => idleLink.evaluate((element) => {
    const style = getComputedStyle(element);
    return { background: style.backgroundColor, color: style.color };
  })).not.toEqual(idleStyle);

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
    const control = getComputedStyle(inner);
    const shell = document.querySelector<HTMLElement>(".console-inner");
    const shellStyle = shell ? getComputedStyle(shell) : null;
    return {
      transform: control.transform,
      transitionDuration: control.transitionDuration,
      animationName: control.animationName,
      shellAnimationName: shellStyle?.animationName ?? "none"
    };
  });
  await page.mouse.up();
  expect(reduced.transform).toBe("none");
  expect(reduced.transitionDuration).toBe("0s");
  expect(reduced.animationName).toBe("none");
  expect(reduced.shellAnimationName).toBe("none");
});
