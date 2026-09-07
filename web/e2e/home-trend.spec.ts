import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import { parseSync, type ESTree } from "vite";
import { selectAppOption } from "./app-select";
import { installSpectrumClock, mockSpectrumTransport, openSpectrumRoute } from "./spectrum-fixtures";

const trendFile = new URL("../src/features/home/HomeTrend.tsx", import.meta.url);

function visualViolations(source: string): string[] {
  const parsed = parseSync("HomeTrend.tsx", source, { lang: "tsx" });
  expect(parsed.errors).toEqual([]);
  const violations: string[] = [];
  const visuals = /^(?:gap|rowGap|columnGap|padding\w*|margin\w*|fontSize|lineHeight|letterSpacing|strokeWidth|strokeDasharray|border\w*Width|borderRadius|minWidth|maxWidth|minHeight|maxHeight|width|height|size)$/;
  const visit = (node: ESTree.Node) => {
    if (node.type === "Literal" && typeof node.value === "string" &&
      /(?:#[\da-f]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch)\s*\(|^(?:red|blue|green|black|white|transparent)$)/i.test(node.value)) {
      violations.push(`color literal: ${node.value}`);
    }
    if (node.type === "JSXOpeningElement" && node.name.type === "JSXIdentifier" &&
      /^(?:svg|path|rect|line|circle|text|g)$/.test(node.name.name)) {
      violations.push(`authored chart geometry: ${node.name.name}`);
    }
    if (node.type === "Property" && node.key.type === "Identifier" && visuals.test(node.key.name)) {
      const values = node.value.type === "ObjectExpression"
        ? node.value.properties.map((property) => property.type === "Property" ? property.value : property)
        : [node.value];
      for (const value of values) {
        const intrinsic = value.type === "Literal" && [0, "full", "auto", "container", "min-content", "max-content"].includes(value.value);
        const scaled = value.type === "TemplateLiteral" && value.quasis.map((part) => part.value.raw).join("") === "[]" &&
          value.expressions.length === 1 && value.expressions[0].type === "CallExpression" &&
          value.expressions[0].callee.type === "Identifier" && value.expressions[0].callee.name === "size";
        let origin = value.type === "TSAsExpression" ? value.expression : value;
        if (origin.type === "ChainExpression") origin = origin.expression;
        while (origin.type === "MemberExpression") origin = origin.object;
        const themed = origin.type === "Identifier" && origin.name === "theme";
        if (!intrinsic && !scaled && !themed) violations.push(`non-Spectrum dimension: ${source.slice(node.start, node.end)}`);
      }
    }
    for (const value of Object.values(node)) {
      for (const child of Array.isArray(value) ? value : [value]) {
        if (child && typeof child === "object" && typeof child.type === "string") visit(child);
      }
    }
  };
  visit(parsed.program);
  return violations;
}

test("home chart visuals come from Spectrum rather than authored colors or geometry", () => {
  expect(visualViolations(readFileSync(trendFile, "utf8"))).toEqual([]);
});

test("chart visual gate rejects color and unscaled spacing regressions", () => {
  for (const source of [
    'const x = {color: "#fff"};',
    'const x = {gap: 17};',
    'const x = {fontSize: "13px"};',
    'const x = <svg><circle r="5" /></svg>;'
  ]) expect(visualViolations(source)).not.toEqual([]);
});

test("healthy attention content shares the section left edge", async ({ page }) => {
  await mockSpectrumTransport(page, { role: "manager" });
  await page.route("**/api/chats/*/queue", (route) => route.fulfill({ json: { items: [] } }));
  await openSpectrumRoute(page, "/home");
  const empty = page.locator("[data-home-attention-empty]");
  await expect(empty).toBeVisible();
  const geometry = await empty.evaluate((element) => {
    const section = element.closest('[data-home-section="attention"]')!;
    const heading = section.querySelector("h2")!.getBoundingClientRect();
    return { left: heading.left, children: Array.from(element.children).map((child) => child.getBoundingClientRect().left) };
  });
  expect(geometry.children).toHaveLength(2);
  for (const left of geometry.children) expect(Math.abs(left - geometry.left)).toBeLessThanOrEqual(1);
});

test("chart date picker exposes complete readings through keyboard selection", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page);
  await openSpectrumRoute(page, "/home");
  const reader = page.getByRole("button", { name: "Choose a date to read values" });
  await reader.focus();
  await page.keyboard.press("Space");
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page.locator("[data-home-chart-reading]")).toHaveText("Aug 27: 12 challenges, 58% pass rate");
  await expect(reader).toBeFocused();
});

test("missing chart days keep the exact returned date domain and explain coverage", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page, { trend: "gap" });
  await openSpectrumRoute(page, "/home");

  const chart = page.getByTestId("home-combined-chart");
  const axisLabels = chart.locator('[aria-label^="X-axis"] .role-axis-label text');
  await expect(axisLabels).toHaveText(["8/26", "8/27", "8/29", "8/30", "8/31", "9/1"]);
  await expect(axisLabels).toHaveCount(6);
  await expect(chart.locator(".mark-text.role-mark text")).toHaveCount(12);
  await expect(chart.locator("svg")).toBeVisible();
  await expect(page.locator("[data-home-trend-coverage]")).toContainText("6 of 7");
  await expect(page.locator("[data-home-chart-reading]")).toContainText("Aug 26");
  expect(await chart.innerHTML()).not.toContain("NaN");
});

test("single-day responses use a single-day chart domain", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page, { trend: "single" });
  await openSpectrumRoute(page, "/home");

  const chart = page.getByTestId("home-combined-chart");
  await expect(chart.locator('[aria-label^="X-axis"] .role-axis-label text')).toHaveText(["9/1"]);
  await expect(chart.locator(".mark-text.role-mark text")).toHaveText(["8", "75%"]);
  await expect(page.locator("[data-home-trend-coverage]")).toContainText("1 of 7");
  await expect(page.locator("[data-home-chart-reading]")).toHaveText("Sep 1: 8 challenges, 75% pass rate");
});


test("native legend dots match the combined chart series across explicit and system themes", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page);
  await openSpectrumRoute(page, "/home");
  const colors: Record<string, string> = {};
  for (const [preference, system] of [["system", "light"], ["system", "dark"], ["light", "dark"], ["dark", "light"]] as const) {
    await page.emulateMedia({ colorScheme: system });
    await selectAppOption(page.getByRole("button", { name: /Theme$/ }), preference);
    const theme = preference === "system" ? system : preference;
    const chart = page.getByTestId("home-combined-chart");
    const labels = chart.locator(".mark-text.role-mark text");
    await expect(labels).toHaveCount(14);
    const bars = chart.locator(".mark-rect.role-mark path").first();
    const line = chart.locator(".mark-line path").first();
    if (colors[theme]) await expect(bars).toHaveAttribute("fill", colors[theme]);
    else if (theme === "dark") await expect(bars).not.toHaveAttribute("fill", colors.light);
    const countColor = await bars.getAttribute("fill");
    const rateColor = await line.getAttribute("stroke");
    expect(countColor).toBeTruthy();
    expect(rateColor).toBeTruthy();
    colors[theme] = countColor!;

    const legend = chart.locator(".role-legend");
    await expect(legend).toContainText(/Requests.*left axis/);
    await expect(legend).toContainText(/Pass rate.*right axis/);
    const symbols = legend.locator(".role-legend-symbol path");
    await expect(symbols).toHaveCount(2);
    await expect(symbols.nth(0)).toHaveAttribute("fill", countColor!);
    await expect(symbols.nth(1)).toHaveAttribute("fill", rateColor!);
    expect(countColor).not.toBe(rateColor);

    const surface = await chart.evaluate((element) => {
      const rgba = (css: string) => css.match(/[\d.]+/g)!.map(Number);
      const luminance = (channels: number[]) => channels.slice(0, 3).map((channel) => {
        const value = channel / 255;
        return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
      }).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
      let background = NaN;
      for (let node: Element | null = element.querySelector("svg"); node; node = node.parentElement) {
        const color = rgba(getComputedStyle(node).backgroundColor);
        if (color.length === 3 || color[3] === 1) { background = luminance(color); break; }
      }
      const bars = Array.from(element.querySelectorAll(".mark-rect.role-mark path")).map((bar) => ({
        box: bar.getBoundingClientRect(),
        luminance: luminance(rgba(getComputedStyle(bar).fill))
      }));
      const contrasts = Array.from(element.querySelectorAll("svg text")).flatMap((label) => {
        const foreground = luminance(rgba(getComputedStyle(label).fill));
        const box = label.getBoundingClientRect();
        const behind = bars.filter(({ box: bar }) =>
          bar.left < box.right && bar.right > box.left && bar.top < box.bottom && bar.bottom > box.top);
        const covered = behind.some(({ box: bar }) =>
          bar.left <= box.left && bar.right >= box.right && bar.top <= box.top && bar.bottom >= box.bottom);
        const backgrounds = [...(covered ? [] : [background]), ...behind.map((bar) => bar.luminance)];
        return backgrounds.map((value) => (Math.max(value, foreground) + 0.05) / (Math.min(value, foreground) + 0.05));
      });
      return { background, contrast: Math.min(...contrasts) };
    });
    if (theme === "dark") expect(surface.background).toBeLessThan(0.2);
    else expect(surface.background).toBeGreaterThan(0.8);
    expect(surface.contrast).toBeGreaterThanOrEqual(4.5);
    const symbol = symbols.first();
    await symbol.scrollIntoViewIfNeeded();
    await expect(symbol).toBeVisible();
    const box = await symbol.boundingBox();
    expect(box!.width).toBeGreaterThan(0);
    expect(Math.abs(box!.width - box!.height)).toBeLessThan(1);
  }
});
