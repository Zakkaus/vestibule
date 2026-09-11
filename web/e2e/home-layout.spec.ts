import { expect, test, type Page } from "@playwright/test";
import { installSpectrumClock, mockSpectrumTransport, openSpectrumRoute } from "./spectrum-fixtures";

const sectionOrder = ["overview", "attention", "trend", "entries"] as const;

async function openHome(page: Page, width: number): Promise<void> {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await page.setViewportSize({ width, height: 720 });
  await installSpectrumClock(page);
  await mockSpectrumTransport(page);
  await openSpectrumRoute(page, "/home");
}

test("desktop home keeps equal section widths and aligned column starts", async ({ page }) => {
  await openHome(page, 1280);

  const sections = page.locator("[data-home-section]");
  await expect(sections).toHaveCount(4);
  const layout = await sections.evaluateAll((elements) => elements.map((element) => {
    const box = element.getBoundingClientRect();
    return {
      id: element.getAttribute("data-home-section"),
      left: box.left,
      top: box.top,
      bottom: box.bottom,
      width: box.width
    };
  }));
  // The grid's row gap steps down on short windows; the sections must sit exactly one gap apart.
  const rowGap = await sections.first().evaluate((element) => {
    const grid = element.parentElement;
    if (!grid) {
      throw new Error("home section has no grid parent");
    }
    return Number.parseFloat(getComputedStyle(grid).rowGap);
  });
  expect([20, 32]).toContain(rowGap);
  const widths = layout.map(({ width }) => width);
  expect(Math.max(...widths) - Math.min(...widths)).toBeLessThanOrEqual(1);
  expect(Math.abs(layout[0].top - layout[1].top)).toBeLessThanOrEqual(1);
  expect(Math.abs(layout[2].top - layout[3].top)).toBeLessThanOrEqual(1);
  expect(Math.abs(layout[0].left - layout[2].left)).toBeLessThanOrEqual(1);
  expect(Math.abs(layout[1].left - layout[3].left)).toBeLessThanOrEqual(1);
  expect(Math.abs(layout[2].top - Math.max(layout[0].bottom, layout[1].bottom) - rowGap)).toBeLessThanOrEqual(1);
});

for (const width of [390, 320] as const) {
  test(`narrow home preserves the overview, attention, trend, entries order at ${width}px`, async ({ page }) => {
    await openHome(page, width);

    const sections = page.locator("[data-home-section]");
    await expect(sections).toHaveCount(4);
    const layout = await sections.evaluateAll((elements) => ({
      ids: elements.map((element) => element.getAttribute("data-home-section")),
      boxes: elements.map((element) => {
        const box = element.getBoundingClientRect();
        return { top: box.top, bottom: box.bottom };
      }),
      documentWidth: document.documentElement.scrollWidth,
      viewportWidth: document.documentElement.clientWidth
    }));

    expect(layout.ids).toEqual(sectionOrder);
    for (const [index, box] of layout.boxes.entries()) {
      if (index > 0) expect(box.top).toBeGreaterThan(layout.boxes[index - 1].bottom);
    }
    expect(layout.documentWidth).toBeLessThanOrEqual(layout.viewportWidth);
  });
}

test("trend plot, semantic legend, and reading controls stay in one bounded region", async ({ page }) => {
  await openHome(page, 1280);

  const chart = page.locator("[data-home-trend-chart]");
  const scroll = chart.locator("[data-home-trend-scroll]");
  const existingReading = chart.locator("[data-home-chart-reading]");
  await expect(scroll).toBeVisible();
  await expect(existingReading).toBeVisible();

  const plot = chart.locator("[data-home-trend-plot]");
  const legend = chart.locator("[data-home-trend-legend]");
  const controls = chart.locator("[data-home-trend-controls]");
  await expect(plot).toBeVisible();
  await expect(legend).toBeVisible();
  await expect(controls).toBeVisible();

  const structure = await chart.evaluate((element) => {
    const region = element.getBoundingClientRect();
    const box = (selector: string) => {
      const child = element.querySelector<HTMLElement>(selector);
      if (!child) throw new Error(`Missing ${selector} after visibility assertion`);
      const value = child.getBoundingClientRect();
      return { left: value.left, right: value.right, top: value.top, bottom: value.bottom };
    };
    const legendItems = Array.from(element.querySelectorAll<HTMLElement>("[data-home-trend-series]")).map((item) => {
      const value = item.getBoundingClientRect();
      return { top: value.top, bottom: value.bottom };
    });
    const picker = element.querySelector<HTMLElement>("[data-home-trend-controls] button");
    const reading = element.querySelector<HTMLElement>("[data-home-chart-reading]");
    if (!picker || !reading) throw new Error("Missing picker/readout after visibility assertion");
    const pickerBox = picker.getBoundingClientRect();
    const readingBox = reading.getBoundingClientRect();
    return {
      region: { left: region.left, right: region.right, top: region.top, bottom: region.bottom },
      plot: box("[data-home-trend-plot]"),
      legend: box("[data-home-trend-legend]"),
      controls: box("[data-home-trend-controls]"),
      legendItems,
      controlsCenter: {
        picker: (pickerBox.top + pickerBox.bottom) / 2,
        reading: (readingBox.top + readingBox.bottom) / 2
      }
    };
  });

  expect(structure.plot.top).toBeLessThan(structure.legend.top);
  expect(structure.legend.top).toBeLessThan(structure.controls.top);
  expect(Math.abs((structure.plot.right - structure.plot.left) - (structure.region.right - structure.region.left))).toBeLessThanOrEqual(1);
  for (const child of [structure.plot, structure.legend, structure.controls]) {
    expect(child.left).toBeGreaterThanOrEqual(structure.region.left - 1);
    expect(child.right).toBeLessThanOrEqual(structure.region.right + 1);
    expect(child.top).toBeGreaterThanOrEqual(structure.region.top - 1);
    expect(child.bottom).toBeLessThanOrEqual(structure.region.bottom + 1);
  }
  expect(structure.legendItems).toHaveLength(2);
  expect(Math.max(...structure.legendItems.map(({ top }) => top)) - Math.min(...structure.legendItems.map(({ top }) => top))).toBeLessThanOrEqual(1);
  expect(Math.abs(structure.controlsCenter.picker - structure.controlsCenter.reading)).toBeLessThanOrEqual(1);
});

for (const width of [390, 320] as const) {
  test(`narrow trend legend and reading controls wrap without page overflow at ${width}px`, async ({ page }) => {
    await openHome(page, width);

    const scroll = page.locator("[data-home-trend-scroll]");
    const existingReading = page.locator("[data-home-chart-reading]");
    await expect(scroll).toBeVisible();
    await expect(existingReading).toBeVisible();
    const plot = page.locator("[data-home-trend-plot]");
    const legend = page.locator("[data-home-trend-legend]");
    const controls = page.locator("[data-home-trend-controls]");
    await expect(plot).toBeVisible();
    await expect(legend).toBeVisible();
    await expect(controls).toBeVisible();

    const geometry = await page.evaluate(() => {
      const region = document.querySelector<HTMLElement>("[data-home-trend-chart]")?.getBoundingClientRect();
      const targets = ["[data-home-trend-plot]", "[data-home-trend-legend]", "[data-home-trend-controls]", "[data-home-chart-reading]"]
        .map((selector) => document.querySelector<HTMLElement>(selector)?.getBoundingClientRect());
      if (!region || targets.some((target) => !target)) throw new Error("Missing trend geometry after visibility assertions");
      return {
        region: { left: region.left, right: region.right },
        targets: targets.map((target) => ({ left: target!.left, right: target!.right })),
        documentWidth: document.documentElement.scrollWidth,
        viewportWidth: document.documentElement.clientWidth
      };
    });

    expect(geometry.documentWidth).toBeLessThanOrEqual(geometry.viewportWidth);
    for (const target of geometry.targets) {
      expect(target.left).toBeGreaterThanOrEqual(geometry.region.left - 1);
      expect(target.right).toBeLessThanOrEqual(geometry.region.right + 1);
    }
  });
}
