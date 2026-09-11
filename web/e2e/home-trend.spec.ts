import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { parseSync, type ESTree } from "vite";
import { selectAppOption } from "./app-select";
import {
  chartDays,
  installSpectrumClock,
  mockSpectrumTransport,
  openSpectrumRoute
} from "./spectrum-fixtures";

const trendFile = new URL("../src/features/home/HomeTrend.tsx", import.meta.url);
type TrendFixtureDay = Readonly<{
  date: string;
  challenges: number;
  passRate: number;
}>;

const completeTrend = chartDays.map(({ date, challenges, passRate }) => ({ date, challenges, passRate }));
const partialTrend: readonly TrendFixtureDay[] = [
  { date: "2026-08-26", challenges: 8, passRate: 0.5 },
  { date: "2026-09-01", challenges: 8, passRate: 0.75 }
];

const coverageText = {
  "zh-CN": "有 5 天没有记录，趋势图仅显示已有日期。",
  "zh-TW": "有 5 天沒有記錄，趨勢圖僅顯示已有日期。",
  en: "Days without readings: 5. Only dates with readings are shown."
} as const;

const emptyCoverageText = {
  "zh-CN": "有 7 天没有记录，趋势图仅显示已有日期。",
  "zh-TW": "有 7 天沒有記錄，趨勢圖僅顯示已有日期。",
  en: "Days without readings: 7. Only dates with readings are shown."
} as const;
const emptyTrendText = {
  "zh-CN": "所选日期范围内没有可用记录。",
  "zh-TW": "所選日期範圍內沒有可用記錄。",
  en: "No readings are available for the selected date range."
} as const;

function trendResponse(trend: readonly TrendFixtureDay[]) {
  return {
    range: { from: "2026-08-26", to: "2026-09-02", timezone: "UTC" },
    summary: { challenges: 70, approved: 41, declined: 15, banned: 4, expired: 10, pass_rate: 0.586 },
    trend: trend.map(({ date, challenges, passRate }) => ({
      date,
      challenges,
      approved: Math.floor(challenges * passRate),
      declined: challenges - Math.floor(challenges * passRate),
      banned: 0,
      expired: 0,
      pass_rate: passRate
    })),
    interceptions: []
  };
}

async function mockTrendResponse(page: Page, trend: readonly TrendFixtureDay[]): Promise<void> {
  await page.route((url) => url.pathname.endsWith("/stats"), (route) => route.fulfill({ json: trendResponse(trend) }));
}

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
  await expect(chart.locator(".role-mark.requestsDirectLabel0 text")).toHaveCount(6);
  await expect(chart.locator("svg")).toBeVisible();
  await expect(page.locator("[data-home-trend-coverage]")).toContainText("Days without readings: 1.");
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
  await expect(chart.locator(".role-mark.requestsDirectLabel0 text")).toHaveText(["8"]);
  await expect(page.locator("[data-home-trend-coverage]")).toContainText("Days without readings: 6.");
  await expect(page.locator("[data-home-chart-reading]")).toHaveText("Sep 1: 8 challenges, 75% pass rate");
});

for (const locale of ["zh-CN", "zh-TW", "en"] as const) {
  test(`complete seven-day responses hide the missing-days notice in ${locale}`, async ({ page }) => {
    await page.addInitScript((value) => localStorage.setItem("verify-console-locale", value), locale);
    await installSpectrumClock(page);
    await mockSpectrumTransport(page);
    await mockTrendResponse(page, completeTrend);
    await openSpectrumRoute(page, "/home");

    await expect(page.locator("[data-home-trend-coverage]")).toHaveCount(0);
    await expect(page.locator("[data-home-trend-empty]")).toHaveCount(0);
    await expect(page.getByTestId("home-combined-chart").locator("svg")).toBeVisible();
  });

  test(`complete zero-activity responses hide the missing-days notice in ${locale}`, async ({ page }) => {
    await page.addInitScript((value) => localStorage.setItem("verify-console-locale", value), locale);
    await installSpectrumClock(page);
    await mockSpectrumTransport(page, { trend: "zero" });
    await openSpectrumRoute(page, "/home");

    await expect(page.locator("[data-home-trend-coverage]")).toHaveCount(0);
    await expect(page.locator("[data-home-trend-empty]")).toHaveCount(0);
    await expect(page.getByTestId("home-combined-chart").locator(".role-mark.requestsDirectLabel0 text")).toHaveText(
      chartDays.map(() => "0")
    );
  });

  test(`partial responses describe exactly the missing dates in ${locale}`, async ({ page }) => {
    await page.addInitScript((value) => localStorage.setItem("verify-console-locale", value), locale);
    await installSpectrumClock(page);
    await mockSpectrumTransport(page);
    await mockTrendResponse(page, partialTrend);
    await openSpectrumRoute(page, "/home");
    const chart = page.getByTestId("home-combined-chart");
    await expect(chart.locator('[aria-label^="X-axis"] .role-axis-label text')).toHaveText(
      partialTrend.map(({ date }) => new Intl.DateTimeFormat(locale, {
        timeZone: "UTC",
        month: "numeric",
        day: "numeric"
      }).format(new Date(`${date}T00:00:00Z`)))
    );

    await expect(page.locator("[data-home-trend-coverage]")).toHaveText(coverageText[locale]);
    await expect(page.locator("[data-home-trend-empty]")).toHaveCount(0);
    await expect(page.getByTestId("home-combined-chart").locator(".role-mark.requestsDirectLabel0 text")).toHaveCount(2);
  });

  test(`empty responses explain the full missing range in ${locale}`, async ({ page }) => {
    await page.addInitScript((value) => localStorage.setItem("verify-console-locale", value), locale);
    await installSpectrumClock(page);
    await mockSpectrumTransport(page);
    await mockTrendResponse(page, []);
    await openSpectrumRoute(page, "/home");

    const empty = page.locator("[data-home-trend-empty]");
    await expect(empty).toBeVisible();
    await expect(empty).toHaveText(emptyTrendText[locale]);
    await expect(page.locator("[data-home-trend-coverage]")).toHaveText(emptyCoverageText[locale]);
    await expect(empty.locator("[data-icon-name]")).toHaveAttribute("data-icon-name", "chartNoAxesCombined");
    await expect(page.getByTestId("home-combined-chart")).toHaveCount(0);
  });

}

test("semantic legend names both chart series without native legend or axis titles", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page);
  await openSpectrumRoute(page, "/home");

  const chart = page.getByTestId("home-combined-chart");
  await expect(chart.locator("svg")).toBeVisible();
  await expect(chart.locator(".role-legend")).toHaveCount(0);
  await expect(chart.locator(".role-axis-title")).toHaveCount(0);

  const legend = page.locator("[data-home-trend-legend]");
  await expect(legend).toBeVisible();
  await expect(legend.locator("[data-home-trend-series]")).toHaveCount(2);
  await expect(legend.locator("[data-home-trend-series='count'] [data-icon-name]")).toHaveAttribute("data-icon-name", "chartColumn");
  await expect(legend.locator("[data-home-trend-series='rate'] [data-icon-name]")).toHaveAttribute("data-icon-name", "chartLine");
  await expect(legend).toContainText(/Requests.*left axis/);
  await expect(legend).toContainText(/Pass rate.*right axis/);
  await expect(page.locator("[data-home-trend-controls]")).toBeVisible();
});

test("combined chart keeps distinct themed colors, surface contrast, and semantic legend colors", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("verify-console-locale", "en"));
  await installSpectrumClock(page);
  await mockSpectrumTransport(page);
  await openSpectrumRoute(page, "/home");

  const legend = page.locator("[data-home-trend-legend]");
  await expect(legend).toBeVisible();
  const colors: Record<string, string> = {};
  for (const [preference, system] of [["system", "light"], ["system", "dark"], ["light", "dark"], ["dark", "light"]] as const) {
    await page.emulateMedia({ colorScheme: system });
    await selectAppOption(page.getByRole("button", { name: /Theme$/ }), preference);
    await expect(legend).toBeVisible();
    await expect(legend.locator("[data-home-trend-series='count'] [data-icon-name]")).toHaveCount(1);
    await expect(legend.locator("[data-home-trend-series='rate'] [data-icon-name]")).toHaveCount(1);

    const theme = preference === "system" ? system : preference;
    const chart = page.getByTestId("home-combined-chart");
    await expect(chart.locator("svg")).toBeVisible();
    const bars = chart.locator(".mark-rect.role-mark path").first();
    const line = chart.locator(".mark-line path").first();
    const countColor = await bars.getAttribute("fill");
    const rateColor = await line.getAttribute("stroke");
    expect(countColor).toBeTruthy();
    expect(rateColor).toBeTruthy();
    expect(countColor).not.toBe(rateColor);
    if (colors[theme]) expect(countColor).toBe(colors[theme]);
    colors[theme] = countColor!;

    const matchingColors = await page.evaluate(() => {
      const normalize = (value: string): string => {
        const probe = document.createElement("span");
        probe.style.color = value;
        document.body.append(probe);
        const normalized = getComputedStyle(probe).color;
        probe.remove();
        return normalized;
      };
      const chartElement = document.querySelector("[data-testid='home-combined-chart']");
      const legendElement = document.querySelector("[data-home-trend-legend]");
      const bar = chartElement?.querySelector<SVGElement>(".mark-rect.role-mark.requests path");
      const line = chartElement?.querySelector<SVGElement>(".mark-line path");
      const countIcon = legendElement?.querySelector<HTMLElement>("[data-home-trend-series='count'] [data-icon-name]");
      const rateIcon = legendElement?.querySelector<HTMLElement>("[data-home-trend-series='rate'] [data-icon-name]");
      if (!bar || !line || !countIcon || !rateIcon) throw new Error("Missing chart or semantic legend colors");
      return {
        chart: {
          count: normalize(getComputedStyle(bar).fill),
          rate: normalize(getComputedStyle(line).stroke)
        },
        legend: {
          count: normalize(getComputedStyle(countIcon).color),
          rate: normalize(getComputedStyle(rateIcon).color)
        }
      };
    });
    expect(matchingColors.legend).toEqual(matchingColors.chart);

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
      // The library draws a transparent hover area and a white backing rect alongside the
      // bars; both are .mark-rect.role-mark, and counting them made every label look like
      // it sat on black. Only the bars themselves are behind a label.
      const bars = Array.from(element.querySelectorAll(".mark-rect.role-mark.requests path")).map((bar) => ({
        box: bar.getBoundingClientRect(),
        luminance: luminance(rgba(getComputedStyle(bar).fill))
      }));
      const contrasts = Array.from(element.querySelectorAll("svg text")).flatMap((label) => {
        const fill = rgba(getComputedStyle(label).fill);
        // Each bar label is drawn twice: an invisible halo behind the visible glyphs.
        // A fully transparent fill puts no ink on the screen and has no contrast to check.
        if (fill.length === 4 && fill[3] === 0) return [];
        const foreground = luminance(fill);
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
  }
});
