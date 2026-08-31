import { expect, test, type Page } from "@playwright/test";

import {
  focusedElement,
  horizontalGeometry,
  measureLocaleWidths,
  renderCell,
  renderWidths,
  themeSurface,
  type FocusObservation,
  type FocusTarget,
  type RenderCell,
  type ThemePreference,
  type ThemeSurface,
  visiblePlaceholderText,
  visibleTabStops,
  widestMeasuredLocale
} from "./render-gate-audits";
import {
  readLocaleCatalogues,
  readRenderRoutes,
  readThemePreferences
} from "./render-gate-routes";

const routes = readRenderRoutes();
const catalogues = readLocaleCatalogues();
const configuredThemes = readThemePreferences();
const requiredThemes: readonly ThemePreference[] = ["light", "dark", "system"];
const unsupportedThemes = configuredThemes.filter(
  (theme) => !requiredThemes.includes(theme as ThemePreference)
);
const missingThemes = requiredThemes.filter((theme) => !configuredThemes.includes(theme));

if (unsupportedThemes.length > 0 || missingThemes.length > 0) {
  throw new Error(
    `Theme registry must contain only light, dark, and system; missing=${missingThemes.join(",") || "none"}, unsupported=${unsupportedThemes.join(",") || "none"}`
  );
}

function matrixCells(): RenderCell[] {
  const cells: RenderCell[] = [];

  for (const route of routes) {
    for (const width of renderWidths) {
      for (const theme of requiredThemes) {
        cells.push({ route, width, theme });
      }
    }
  }

  return cells;
}

async function widestLocaleFor(page: Page): Promise<string> {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");
  await page.locator("[data-app-shell]").waitFor({ state: "visible" });

  const measurements = await measureLocaleWidths(page, catalogues);
  const widest = widestMeasuredLocale(measurements);
  console.log(
    `[render-gate] locale widths: ${measurements
      .map(
        (measurement) =>
          `${measurement.locale}=${measurement.totalWidth.toFixed(2)}px/${measurement.widestMessageWidth.toFixed(2)}px`
      )
      .join(", ")}; selected=${widest.locale}`
  );
  return widest.locale;
}

function themeProblems(
  surface: ThemeSurface,
  cell: RenderCell,
  lightSurface: ThemeSurface | undefined
): string[] {
  const problems: string[] = [];
  const expectedDataTheme = cell.theme === "system" ? null : cell.theme;

  if (surface.dataTheme !== expectedDataTheme) {
    problems.push(
      `data-theme=${surface.dataTheme ?? "absent"}, expected ${expectedDataTheme ?? "absent"}`
    );
  }
  if (surface.dataThemePreference !== cell.theme) {
    problems.push(
      `data-theme-preference=${surface.dataThemePreference ?? "absent"}, expected ${cell.theme}`
    );
  }

  for (const [name, color, directlyDeclared] of [
    ["background", surface.background, surface.bodyDeclaresBackground],
    ["foreground", surface.foreground, surface.bodyDeclaresForeground]
  ] as const) {
    if (!directlyDeclared) {
      problems.push(`body ${name} is inherited instead of declared on body`);
    }
    if (!color.rgba) {
      problems.push(`body ${name} does not resolve to a CSS color: ${color.css}`);
      continue;
    }
    if (color.rgba.at(3) !== 255) {
      problems.push(`body ${name} is transparent: ${color.css}`);
    }
  }

  if (cell.theme !== "light") {
    if (!lightSurface) {
      problems.push("missing light-theme baseline for surface comparison");
    } else {
      for (const [name, color, lightColor] of [
        ["background", surface.background.rgba, lightSurface.background.rgba],
        ["foreground", surface.foreground.rgba, lightSurface.foreground.rgba]
      ] as const) {
        if (color && lightColor && color.join(",") === lightColor.join(",")) {
          problems.push(`dark ${name} equals the explicit-light ${name}`);
        }
      }
    }
  }

  return problems;
}

function focusProblems(observation: FocusObservation): string[] {
  const problems: string[] = [];
  const target = observation.target;

  if (!target) {
    return ["Tab left the document before every visible tab stop was visited"];
  }
  if (!observation.visible) {
    problems.push(`${target.tagName} ${target.name} is not visibly rendered`);
  }
  const visibleOutline =
    observation.outlineStyle !== "none" &&
    observation.outlineWidth > 0 &&
    observation.outlineColor?.at(3) !== 0;
  if (!visibleOutline && observation.boxShadow === "none") {
    problems.push(
      `${target.tagName} ${target.name} has neither a visible outline nor a focus shadow`
    );
  }

  return problems;
}

test("render gate rejects page overflow while allowing scoped horizontal scrollers", async ({
  page
}) => {
  const widestLocale = await widestLocaleFor(page);

  for (const cell of matrixCells()) {
    await test.step(`${cell.route.sourcePath} at ${cell.width}px / ${cell.theme}`, async () => {
      await renderCell(page, cell, widestLocale);
      const geometry = await horizontalGeometry(page);
      const cellName = `${cell.route.sourcePath} (${cell.width}px, ${cell.theme})`;

      expect(geometry.document.scrollWidth, `${cellName}: page horizontal overflow`).toBeLessThanOrEqual(
        geometry.document.clientWidth
      );
      expect(geometry.escapedElements, `${cellName}: overflow escaped every scoped scroller`).toEqual(
        []
      );
      expect(
        geometry.scopedScrollersOutsideViewport,
        `${cellName}: scoped scroller extends beyond the page viewport`
      ).toEqual([]);
    });
  }
});

test("render gate rejects visible placeholders and unresolved i18n keys", async ({ page }) => {
  const widestLocale = await widestLocaleFor(page);

  for (const cell of matrixCells()) {
    await test.step(`${cell.route.sourcePath} at ${cell.width}px / ${cell.theme}`, async () => {
      await renderCell(page, cell, widestLocale);
      const cellName = `${cell.route.sourcePath} (${cell.width}px, ${cell.theme})`;

      expect(await visiblePlaceholderText(page), `${cellName}: visible placeholder text`).toEqual([]);
    });
  }
});

test("render gate rejects transparent, inherited, or light-leaking theme surfaces", async ({
  page
}) => {
  const widestLocale = await widestLocaleFor(page);
  const lightSurfaces: Record<string, ThemeSurface | undefined> = {};

  for (const cell of matrixCells()) {
    await test.step(`${cell.route.sourcePath} at ${cell.width}px / ${cell.theme}`, async () => {
      await renderCell(page, cell, widestLocale);
      const surface = await themeSurface(page);
      const baselineKey = `${cell.route.sourcePath}:${cell.width}`;
      const lightSurface = lightSurfaces[baselineKey];
      const cellName = `${cell.route.sourcePath} (${cell.width}px, ${cell.theme})`;

      expect(themeProblems(surface, cell, lightSurface), `${cellName}: theme surface`).toEqual([]);

      if (cell.theme === "light") {
        lightSurfaces[baselineKey] = surface;
      }
    });
  }
});

test("render gate tabs through every queue row action with a visible focus ring", async ({
  page
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/queue");
  await page.locator("[data-queue-page]").waitFor({ state: "visible" });
  await page.evaluate(async () => {
    await document.fonts.ready;
  });

  expect(await page.evaluate(() => document.activeElement?.tagName)).toBe("BODY");

  const expectedStops = await visibleTabStops(page);
  const expectedQueueActions = expectedStops.filter(
    (target) => target.queueActionId !== null && target.queueRowId !== null
  );
  expect(expectedQueueActions, "queue must expose at least one row-end action").not.toEqual([]);

  const visitedQueueActions: FocusTarget[] = [];
  for (const expectedStop of expectedStops) {
    await page.keyboard.press("Tab");
    const observation = await focusedElement(page);

    expect(observation.target?.index, `Tab focus order for ${expectedStop.name}`).toBe(
      expectedStop.index
    );
    expect(focusProblems(observation), `focus ring for ${expectedStop.name}`).toEqual([]);

    if (observation.target?.queueActionId && observation.target.queueRowId) {
      visitedQueueActions.push(observation.target);
    }
  }

  expect(
    visitedQueueActions.map((target) => `${target.queueRowId}:${target.queueActionId}`),
    "Tab traversal must reach every row-end queue action"
  ).toEqual(
    expectedQueueActions.map((target) => `${target.queueRowId}:${target.queueActionId}`)
  );
});
