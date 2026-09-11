import { expect, test, type Page } from "@playwright/test";
import { selectAppOption } from "./app-select";
import { chartDays, mockSpectrumTransport, openSpectrumRoute } from "./spectrum-fixtures";

async function dateLabelOverlaps(page: Page) {
  return page.locator('[data-testid="home-combined-chart"] [aria-label^="X-axis"] .role-axis-label text').evaluateAll((labels) => {
    const boxes = labels.map((label) => ({
      text: label.textContent,
      box: label.getBoundingClientRect().toJSON()
    }));
    const overlaps = [];
    for (let left = 0; left < boxes.length; left++) {
      for (let right = left + 1; right < boxes.length; right++) {
        const a = boxes[left].box;
        const b = boxes[right].box;
        if (Math.min(a.right, b.right) > Math.max(a.left, b.left) &&
            Math.min(a.bottom, b.bottom) > Math.max(a.top, b.top)) {
          overlaps.push([boxes[left], boxes[right]]);
        }
      }
    }
    return { boxes, overlaps };
  });
}

// The side nav's own tree is the scroller; the sidebar around it never scrolls.
const navigationScroller = ".console-sidebar .console-navigation [role='treegrid']";

// How much of the first and last destination the tree's own scrollport shows.
async function endRowsVisible(page: Page) {
  return page.locator(navigationScroller).evaluate((scrollport) => {
    if (!/(auto|scroll)/.test(getComputedStyle(scrollport).overflowY)) {
      throw new Error("the navigation tree is not the scroller");
    }
    const bounds = scrollport.getBoundingClientRect();
    const top = bounds.top + scrollport.clientTop;
    const bottom = top + scrollport.clientHeight;
    const links = [...scrollport.querySelectorAll("[data-navigation-item]")];
    if (links.length !== 15) throw new Error(`expected 15 destinations, found ${links.length}`);
    const visible = (link: Element) => {
      const box = link.getBoundingClientRect();
      return (Math.min(box.bottom, bottom) - Math.max(box.top, top)) / box.height;
    };
    return { first: visible(links[0]!), last: visible(links.at(-1)!), scrollTop: scrollport.scrollTop };
  });
}

test("the navigation tree scrolls to whole rows at both ends", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");
  await expect(page.locator('.console-sidebar a[aria-current="page"]')).toBeInViewport({ ratio: 1 });
  const atTop = await endRowsVisible(page);
  expect(atTop.scrollTop).toBe(0);
  expect(atTop.first).toBe(1);
  // Fifteen rows do not fit 720px, so the last one starts below the fold.
  expect(atTop.last).toBeLessThan(1);

  await page.locator(navigationScroller).evaluate((scrollport) => {
    scrollport.scrollTop = scrollport.scrollHeight;
  });
  const atBottom = await endRowsVisible(page);
  expect(atBottom.scrollTop).toBeGreaterThan(0);
  expect(atBottom.last).toBe(1);
  await expect(page.locator('.console-sidebar [data-navigation-item="/preferences"]')).toBeInViewport({ ratio: 1 });
});


test("the current destination is marked by the side nav's own cues in both themes", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");

  for (const theme of ["light", "dark"] as const) {
    const themeTrigger = page
      .locator('[data-utility-controls][data-variant="chrome"] [data-console-control]')
      .first()
      .locator("button")
      .first();
    await selectAppOption(themeTrigger, theme);
    await page.waitForFunction((expected) => document.documentElement.dataset.theme === expected, theme);
    // The theme change fades colours in several waves, each starting as the previous
    // one lands; wait until no transition is running before measuring.
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
    await page.waitForFunction(() => document.getAnimations().length === 0);

    const cues = await page.locator('.console-sidebar [data-navigation-item]').evaluateAll((links) => {
      const measure = (link: Element) => {
        const svg = link.querySelector("svg");
        const box = link.getBoundingClientRect();
        const row = link.closest("[role='row']")!;
        // The library's current-item indicator: a 2px bar in the text colour, left of the row.
        const rails = [...row.querySelectorAll<HTMLElement>("div")].filter((element) => {
          const rect = element.getBoundingClientRect();
          const css = getComputedStyle(element);
          return rect.width >= 1 && rect.width <= 4 && rect.height >= 12 &&
            rect.right <= box.left && rect.bottom > box.top && rect.top < box.bottom &&
            css.backgroundColor !== "rgba(0, 0, 0, 0)";
        }).map((element) => getComputedStyle(element).backgroundColor);
        return {
          weight: Number(getComputedStyle(link).fontWeight),
          color: getComputedStyle(link).color,
          stroke: svg ? getComputedStyle(svg).strokeWidth : null,
          rails
        };
      };
      const active = links.find((link) => link.getAttribute("aria-current") === "page")!;
      const idle = links.find((link) => !link.hasAttribute("aria-current"))!;
      return { active: measure(active), idle: measure(idle) };
    });
    expect(cues.active.weight).toBe(500);
    expect(cues.idle.weight).toBe(400);
    expect(cues.active.color).not.toBe(cues.idle.color);
    expect(cues.active.stroke).toBe(cues.idle.stroke);
    expect(cues.active.rails).toEqual([cues.active.color]);
    expect(cues.idle.rails).toEqual([]);
    // Text, not just the bar, follows the theme.
    expect(cues.idle.color).toBe(theme === "light" ? "rgb(80, 80, 80)" : "rgb(175, 175, 175)");
  }
});

test("content navigation moves the current marker and the keyboard entry point to the destination", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await page.locator('[data-home-metric="challenges"]').click();
  await expect(page).toHaveURL(/\/stats\?/);
  const destination = page.locator('.console-sidebar [data-navigation-item="/stats"]');
  await expect(destination).toHaveAttribute("aria-current", "page");
  await expect(page.locator('.console-sidebar [aria-current="page"]')).toHaveCount(1);
  await expect(destination).toBeInViewport({ ratio: 1 });
  // Tab from the brand link enters the tree at the current destination, not at the first row.
  await page.locator(".console-brand a").focus();
  await page.keyboard.press("Tab");
  await expect(destination).toBeFocused();
});

for (const locale of ["zh-CN", "zh-TW", "en"] as const) {
  test(`all horizontal combined-chart labels stay disjoint in ${locale}, including an out-of-window response`, async ({ page }) => {
    await page.addInitScript((value) => localStorage.setItem("verify-console-locale", value), locale);
    // The response contains Aug 26–Sep 1 while the requested range starts Aug 31.
    // Returned dates must remain distinct without synthetic blank domain slots.
    await page.clock.setFixedTime(new Date("2026-09-06T12:00:00Z"));
    await mockSpectrumTransport(page, { role: "operator" });
    for (const width of [1280, 390, 320]) {
      await page.setViewportSize({ width, height: 720 });
      await openSpectrumRoute(page, "/home");
      await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");
      await page.evaluate(async () => {
        await document.fonts.ready;
        await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      });
      const { boxes, overlaps } = await dateLabelOverlaps(page);
      expect(boxes.map(({ text }) => text)).toEqual(
        chartDays.map(({ date }) => new Intl.DateTimeFormat(locale, {
          timeZone: "UTC", month: "numeric", day: "numeric"
        }).format(new Date(`${date}T00:00:00Z`)))
      );
      expect(boxes.every(({ box }) => box.width > 0 && box.height > 0)).toBe(true);
      expect(overlaps, `${locale} ${width}px`).toEqual([]);
      const coverage = await page.locator("[data-home-trend-coverage]").innerText();
      expect(coverage.match(/\d+/g)?.map(Number)).toEqual([5]);

      const chartGeometry = await page.getByTestId("home-combined-chart").evaluate((element) => {
        const svg = element.querySelector("svg");
        const scroll = element.closest<HTMLElement>("[data-home-trend-scroll]");
        if (!svg || !scroll) throw new Error("Home chart geometry is missing");
        return {
          svgHeight: svg.getBoundingClientRect().height,
          scrollHeight: scroll.scrollHeight,
          scrollClientHeight: scroll.clientHeight
        };
      });
      expect(chartGeometry.svgHeight).toBeGreaterThan(0);
      expect(chartGeometry.scrollHeight).toBeLessThanOrEqual(chartGeometry.scrollClientHeight + 1);
    }
  });
}

test("queue sorts and selects real rows without changing release eligibility", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/queue");
  const grid = page.getByRole("grid", { name: "Waiting queue records" });
  await expect(grid).toBeVisible();
  const userColumn = grid.getByRole("columnheader", { name: "User", exact: true });
  await userColumn.click();
  await expect(userColumn).toHaveAttribute("aria-sort", "ascending");
  await expect(grid.locator('[data-queue-user]')).toHaveText(["@approved", "@waiting"]);
  const waiting = grid.locator('[data-queue-row="queue-1"]');
  await waiting.locator("label:has(input[type=checkbox])").click();
  await expect(waiting.getByRole("checkbox")).toBeChecked();
  await expect(waiting).toHaveAttribute("aria-selected", "true");
  await userColumn.click();
  await expect(userColumn).toHaveAttribute("aria-sort", "descending");
  await expect(grid.locator('[data-queue-user]')).toHaveText(["@waiting", "@approved"]);
  await expect(waiting).toHaveAttribute("aria-selected", "true");
  await expect(waiting.getByRole("button", { name: "Release @waiting", exact: true })).toBeEnabled();
  await expect(grid.locator('[data-queue-row="queue-2"]').getByRole("button", { name: /Release/ })).toHaveCount(0);
});

test("queue empty states remain within its accessible grid and filters recover rows", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/queue");
  const grid = page.getByRole("grid", { name: "Waiting queue records" });
  await page.getByRole("textbox", { name: "Filter waiting queue" }).fill("no-such-applicant");
  await expect(grid.getByRole("heading", { name: "No records match the current filters" })).toBeVisible();
  await grid.getByRole("button", { name: "Clear filters" }).click();
  await expect(grid.locator('[data-queue-user]')).toHaveText(["@waiting", "@approved"]);
  await page.route("**/queue", async (route) => {
    if (new URL(route.request().url()).pathname.startsWith("/api/")) {
      await route.fulfill({ json: { items: [] } });
    } else await route.fallback();
  });
  await page.reload();
  await expect(grid.getByRole("heading", { name: "No one has applied yet" })).toBeVisible();
  await expect(grid.locator('[data-queue-row]')).toHaveCount(0);
});
