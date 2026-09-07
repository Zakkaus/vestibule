import { expect, test, type Page } from "@playwright/test";
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

async function clippedNavigation(page: Page) {
  return page.locator('.console-sidebar a[href]').evaluateAll((links) => {
    const clipped = [];
    for (const link of links) {
      const box = link.getBoundingClientRect();
      if (!box.width || !box.height) continue;
      for (let parent = link.parentElement; parent && parent !== document.body; parent = parent.parentElement) {
        const overflow = getComputedStyle(parent).overflowY;
        if (!["auto", "scroll", "hidden", "clip"].includes(overflow)) continue;
        const bounds = parent.getBoundingClientRect();
        const top = bounds.top + parent.clientTop;
        const bottom = top + parent.clientHeight;
        const visible = Math.min(box.bottom, bottom) - Math.max(box.top, top);
        if (visible > 0.5 && visible < box.height - 0.5) {
          clipped.push({ label: link.textContent, item: box.toJSON(), boundary: { top, bottom } });
        }
      }
    }
    return clipped;
  });
}

test("sidebar never cuts a navigation item at a scroll boundary", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");
  await expect(page.locator('.console-sidebar a[aria-current="page"]')).toBeVisible();
  expect(await clippedNavigation(page)).toEqual([]);

  const groups = page.locator('.console-sidebar [aria-expanded]');
  for (let index = 0; index < await groups.count(); index++) {
    const group = groups.nth(index);
    if (await group.getAttribute("aria-expanded") === "false") await group.click();
    expect(await clippedNavigation(page)).toEqual([]);
  }
  await page.evaluate(() => {
    for (const element of document.querySelectorAll<HTMLElement>('.console-sidebar *')) {
      if (["auto", "scroll"].includes(getComputedStyle(element).overflowY)) element.scrollTop = element.scrollHeight;
    }
  });
  expect(await clippedNavigation(page)).toEqual([]);
});

test("sidebar surface follows document overflow in a short viewport without a bottom seam", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 480 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await expect(page.locator('[data-home-page]')).toHaveAttribute("data-home-state", "loaded");
  const geometry = await page.locator('.console-sidebar').evaluate((sidebar) => {
    const box = sidebar.getBoundingClientRect();
    const shell = sidebar.closest('[data-app-shell]')!.getBoundingClientRect();
    const css = getComputedStyle(sidebar);
    return { bottom: box.bottom, shellBottom: shell.bottom, viewport: innerHeight, border: css.borderBottomWidth };
  });
  expect(geometry.bottom).toBeGreaterThan(geometry.viewport);
  expect(Math.abs(geometry.bottom - geometry.shellBottom)).toBeLessThanOrEqual(1);
  expect(geometry.border).toBe("0px");
  await page.locator("[data-home-entries]").scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => scrollY)).toBeGreaterThan(0);
  await expect(page.locator('.console-sidebar a[aria-current="page"]')).toBeInViewport({ ratio: 1 });
});

test("active navigation combines weight, an inset rail, and icon stroke without a colored slab", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  const cues = await page.locator('.console-sidebar nav a[href]').evaluateAll((links) => {
    const measure = (link: Element) => {
      const row = link.closest('[role="row"]') ?? link;
      const box = link.getBoundingClientRect();
      const rail = [...row.querySelectorAll<HTMLElement>("div")].some((element) => {
        const rect = element.getBoundingClientRect();
        const css = getComputedStyle(element);
        return rect.width >= 1 && rect.width <= 4 && rect.height >= 12 &&
          rect.right <= box.left && rect.bottom > box.top && rect.top < box.bottom &&
          css.backgroundColor !== "rgba(0, 0, 0, 0)";
      });
      const canvas = document.createElement("canvas");
      canvas.width = canvas.height = 1;
      const context = canvas.getContext("2d")!;
      const saturatedSlab = [row, ...row.querySelectorAll<HTMLElement>("div, a")].some((element) => {
        const rect = element.getBoundingClientRect();
        if (rect.width < box.width / 2 || rect.height < box.height / 2) return false;
        context.clearRect(0, 0, 1, 1);
        context.fillStyle = getComputedStyle(element).backgroundColor;
        context.fillRect(0, 0, 1, 1);
        const [red, green, blue, alpha] = context.getImageData(0, 0, 1, 1).data;
        return alpha > 0 && Math.max(red, green, blue) - Math.min(red, green, blue) > 12;
      });
      return {
        weight: Number(getComputedStyle(link).fontWeight),
        stroke: Number.parseFloat(getComputedStyle(link.querySelector("svg")!).strokeWidth),
        rail,
        saturatedSlab
      };
    };
    const active = links.find((link) => link.getAttribute("aria-current") === "page")!;
    const idle = links.find((link) => !link.hasAttribute("aria-current"))!;
    return { active: measure(active), idle: measure(idle) };
  });
  expect(cues.active.weight).toBeGreaterThan(cues.idle.weight);
  expect(cues.active.stroke).toBeGreaterThan(cues.idle.stroke);
  expect(cues.active.rail).toBe(true);
  expect(cues.idle.rail).toBe(false);
  expect(cues.active.saturatedSlab).toBe(false);
});

test("content navigation opens the destination group and preserves its keyboard entry", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await mockSpectrumTransport(page, { role: "operator" });
  await openSpectrumRoute(page, "/home");
  await page.locator('[data-home-metric="challenges"]').click();
  await expect(page).toHaveURL(/\/stats\?/);
  await expect(page.locator('.console-sidebar [data-navigation-group="observe"]')).toHaveAttribute("aria-expanded", "true");
  const destination = page.locator('.console-sidebar nav a[href^="/stats"]');
  await expect(destination).toBeVisible();
  await page.locator(".console-brand a").focus();
  await page.keyboard.press("Tab");
  await expect(destination).toBeFocused();
  await expect(destination).toHaveAttribute("aria-current", "page");
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
      expect(coverage.match(/\d+/g)?.map(Number)).toEqual([2, 7, 5]);

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
