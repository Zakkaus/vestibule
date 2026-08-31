import type { Page } from "@playwright/test";

import type { LocaleCatalogues, RenderRoute } from "./render-gate-routes";

export const renderWidths = [320, 1280] as const;

export type ThemePreference = "light" | "dark" | "system";

export type RenderCell = Readonly<{
  route: RenderRoute;
  width: (typeof renderWidths)[number];
  theme: ThemePreference;
}>;

export type LocaleMeasurement = Readonly<{
  locale: string;
  messageCount: number;
  totalWidth: number;
  widestMessageWidth: number;
}>;

export type HorizontalGeometry = Readonly<{
  document: Readonly<{
    scrollWidth: number;
    clientWidth: number;
  }>;
  escapedElements: readonly string[];
  scopedScrollersOutsideViewport: readonly string[];
  scopedScrollers: readonly string[];
}>;

export type ThemeSurface = Readonly<{
  dataTheme: string | null;
  dataThemePreference: string | null;
  background: Readonly<{
    css: string;
    rgba: readonly number[] | null;
  }>;
  foreground: Readonly<{
    css: string;
    rgba: readonly number[] | null;
  }>;
  bodyDeclaresBackground: boolean;
  bodyDeclaresForeground: boolean;
}>;

export type FocusTarget = Readonly<{
  index: number;
  tagName: string;
  name: string;
  queueActionId: string | null;
  queueRowId: string | null;
}>;

export type FocusObservation = Readonly<{
  target: FocusTarget | null;
  visible: boolean;
  outlineStyle: string;
  outlineWidth: number;
  outlineColor: readonly number[] | null;
  boxShadow: string;
}>;

export async function measureLocaleWidths(
  page: Page,
  catalogues: LocaleCatalogues
): Promise<LocaleMeasurement[]> {
  const entries = Object.entries(catalogues).map(([locale, messages]) => [
    locale,
    [...messages]
  ]);

  return page.evaluate(async (localeEntries) => {
    await document.fonts.ready;

    const context = document.createElement("canvas").getContext("2d");
    if (!context) {
      throw new Error("Canvas 2D context is unavailable for locale measurement");
    }

    const bodyStyle = getComputedStyle(document.body);
    context.font = `${bodyStyle.fontStyle} ${bodyStyle.fontWeight} ${bodyStyle.fontSize} ${bodyStyle.fontFamily}`;

    return localeEntries.map(([locale, messages]) => {
      const widths = messages.map((message) => context.measureText(message).width);
      return {
        locale,
        messageCount: messages.length,
        totalWidth: widths.reduce((total, width) => total + width, 0),
        widestMessageWidth: Math.max(0, ...widths)
      };
    });
  }, entries);
}

export function widestMeasuredLocale(
  measurements: readonly LocaleMeasurement[]
): LocaleMeasurement {
  const firstMeasurement = measurements.at(0);
  if (!firstMeasurement) {
    throw new Error("Cannot choose a widest locale without message measurements");
  }

  return measurements.slice(1).reduce((widest, candidate) => {
    if (candidate.totalWidth !== widest.totalWidth) {
      return candidate.totalWidth > widest.totalWidth ? candidate : widest;
    }

    if (candidate.widestMessageWidth !== widest.widestMessageWidth) {
      return candidate.widestMessageWidth > widest.widestMessageWidth
        ? candidate
        : widest;
    }

    return candidate.locale.localeCompare(widest.locale) < 0 ? candidate : widest;
  }, firstMeasurement);
}

export async function renderCell(
  page: Page,
  cell: RenderCell,
  locale: string
): Promise<void> {
  await page.setViewportSize({ width: cell.width, height: 900 });
  await page.emulateMedia({
    colorScheme: cell.theme === "system" ? "dark" : "light"
  });
  await page.goto(cell.route.urlPath);
  await page.locator("[data-app-shell]").waitFor({ state: "visible" });

  const controls = page.locator("[data-utility-controls]").first();
  const selects = controls.locator("select");
  if ((await selects.count()) !== 2) {
    throw new Error(`${cell.route.sourcePath}: utility controls must expose theme and locale selects`);
  }

  await selects.nth(0).selectOption(cell.theme);
  await page.waitForFunction((theme) => {
    const root = document.documentElement;
    return (
      root.dataset.themePreference === theme &&
      (theme === "system" ? !root.hasAttribute("data-theme") : root.dataset.theme === theme)
    );
  }, cell.theme);

  await selects.nth(1).selectOption(locale);
  await page.waitForFunction(
    (selectedLocale) => document.documentElement.lang === selectedLocale,
    locale
  );
  await page.evaluate(async () => {
    await document.fonts.ready;
    await new Promise<void>((resolve) => {
      requestAnimationFrame(() => requestAnimationFrame(resolve));
    });
  });
}

export async function horizontalGeometry(page: Page): Promise<HorizontalGeometry> {
  return page.evaluate(() => {
    const root = document.documentElement;
    const viewportWidth = root.clientWidth;
    const overflowingElements = [...document.body.querySelectorAll<HTMLElement>("*")].filter(
      (element) => {
        const overflowX = getComputedStyle(element).overflowX;
        return (
          element.clientWidth > 0 &&
          element.scrollWidth > element.clientWidth + 1 &&
          (overflowX === "visible" || overflowX === "auto" || overflowX === "scroll")
        );
      }
    );
    const selectorFor = (element: HTMLElement): string => {
      const dataName = element
        .getAttributeNames()
        .find((name) => name.startsWith("data-"));
      return `${element.tagName.toLowerCase()}${dataName ? `[${dataName}]` : ""}`;
    };
    const closestScopedScroller = (element: HTMLElement): HTMLElement | null => {
      for (
        let current: HTMLElement | null = element;
        current && current !== document.body;
        current = current.parentElement
      ) {
        const overflowX = getComputedStyle(current).overflowX;
        if (overflowX === "auto" || overflowX === "scroll") {
          return current;
        }
      }
      return null;
    };
    const scopedScrollers = [
      ...new Set(
        overflowingElements
          .map((element) => closestScopedScroller(element))
          .filter((element): element is HTMLElement => element !== null)
      )
    ];

    return {
      document: {
        scrollWidth: root.scrollWidth,
        clientWidth: root.clientWidth
      },
      escapedElements: overflowingElements
        .filter((element) => closestScopedScroller(element) === null)
        .map(selectorFor),
      scopedScrollersOutsideViewport: scopedScrollers
        .filter((element) => {
          const rect = element.getBoundingClientRect();
          return rect.left < -1 || rect.right > viewportWidth + 1;
        })
        .map(selectorFor),
      scopedScrollers: scopedScrollers.map(selectorFor)
    };
  });
}

export async function visiblePlaceholderText(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const placeholder =
      /\b(?:undefined|null|NaN)\b|\[object Object\]|\b[a-z][a-z0-9_-]*(?:\.[a-z][a-z0-9_-]*)+\b/i;

    return [...document.body.querySelectorAll<HTMLElement>("*")]
      .filter((element) => {
        const visibility = element.checkVisibility?.({
          checkOpacity: true,
          checkVisibilityCSS: true
        });
        return !element.children.length && visibility !== false;
      })
      .flatMap((element) => {
        const text = element.textContent?.trim() ?? "";
        if (!text || !placeholder.test(text)) {
          return [];
        }

        const dataName = element
          .getAttributeNames()
          .find((name) => name.startsWith("data-"));
        return [`${element.tagName.toLowerCase()}${dataName ? `[${dataName}]` : ""}: ${text}`];
      });
  });
}

export async function themeSurface(page: Page): Promise<ThemeSurface> {
  return page.evaluate(() => {
    const colorFromCss = (css: string): readonly number[] | null => {
      const context = document.createElement("canvas").getContext("2d", {
        willReadFrequently: true
      });
      if (!context) {
        return null;
      }

      const normalized = (sentinel: string): string => {
        context.fillStyle = sentinel;
        context.fillStyle = css;
        return context.fillStyle;
      };
      if (normalized("#000000") !== normalized("#ffffff")) {
        return null;
      }

      context.clearRect(0, 0, 1, 1);
      context.fillRect(0, 0, 1, 1);
      return [...context.getImageData(0, 0, 1, 1).data];
    };
    const bodyDeclares = (properties: readonly string[]): boolean => {
      const inspectRules = (rules: CSSRuleList): boolean => {
        for (const rule of rules) {
          if (rule.type === CSSRule.STYLE_RULE) {
            const styleRule = rule as CSSStyleRule;
            try {
              if (
                document.body.matches(styleRule.selectorText) &&
                properties.some((property) => styleRule.style.getPropertyValue(property))
              ) {
                return true;
              }
            } catch {
              continue;
            }
          }

          if ("cssRules" in rule && inspectRules(rule.cssRules)) {
            return true;
          }
        }
        return false;
      };

      for (const stylesheet of document.styleSheets) {
        try {
          if (inspectRules(stylesheet.cssRules)) {
            return true;
          }
        } catch {
          continue;
        }
      }
      return false;
    };
    const bodyStyle = getComputedStyle(document.body);
    const background = bodyStyle.backgroundColor;
    const foreground = bodyStyle.color;

    return {
      dataTheme: document.documentElement.getAttribute("data-theme"),
      dataThemePreference: document.documentElement.dataset.themePreference ?? null,
      background: { css: background, rgba: colorFromCss(background) },
      foreground: { css: foreground, rgba: colorFromCss(foreground) },
      bodyDeclaresBackground: bodyDeclares(["background", "background-color"]),
      bodyDeclaresForeground: bodyDeclares(["color"])
    };
  });
}

export async function visibleTabStops(page: Page): Promise<FocusTarget[]> {
  return page.evaluate(() => {
    const candidates = [
      ...document.querySelectorAll<HTMLElement>(
        "a[href], button:not([disabled]), input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])"
      )
    ].filter((element) => {
      const visibility = element.checkVisibility?.({
        checkOpacity: true,
        checkVisibilityCSS: true
      });
      return visibility !== false && element.tabIndex >= 0;
    });
    const orderedCandidates = candidates
      .filter((element) => element.tabIndex > 0)
      .sort((left, right) => left.tabIndex - right.tabIndex)
      .concat(candidates.filter((element) => element.tabIndex === 0));

    return orderedCandidates.map((element, index) => ({
      index,
      tagName: element.tagName.toLowerCase(),
      name:
        element.getAttribute("aria-label") ??
        element.textContent?.replace(/\s+/g, " ").trim() ??
        "",
      queueActionId: element.getAttribute("data-queue-action-id"),
      queueRowId: element.closest("[data-queue-row]")?.getAttribute("data-queue-row") ?? null
    }));
  });
}

export async function focusedElement(page: Page): Promise<FocusObservation> {
  return page.evaluate(() => {
    const candidates = [
      ...document.querySelectorAll<HTMLElement>(
        "a[href], button:not([disabled]), input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex='-1'])"
      )
    ].filter((element) => {
      const visibility = element.checkVisibility?.({
        checkOpacity: true,
        checkVisibilityCSS: true
      });
      return visibility !== false && element.tabIndex >= 0;
    });
    const orderedCandidates = candidates
      .filter((element) => element.tabIndex > 0)
      .sort((left, right) => left.tabIndex - right.tabIndex)
      .concat(candidates.filter((element) => element.tabIndex === 0));
    const activeElement = document.activeElement;
    const targetIndex = orderedCandidates.indexOf(activeElement as HTMLElement);
    const target = targetIndex < 0 ? null : orderedCandidates[targetIndex]!;

    if (!target) {
      return {
        target: null,
        visible: false,
        outlineStyle: "none",
        outlineWidth: 0,
        outlineColor: null,
        boxShadow: "none"
      };
    }

    const computed = getComputedStyle(target);
    const context = document.createElement("canvas").getContext("2d", {
      willReadFrequently: true
    });
    let outlineColor: readonly number[] | null = null;
    if (context) {
      const css = computed.outlineColor;
      const normalized = (sentinel: string): string => {
        context.fillStyle = sentinel;
        context.fillStyle = css;
        return context.fillStyle;
      };
      if (normalized("#000000") === normalized("#ffffff")) {
        context.clearRect(0, 0, 1, 1);
        context.fillRect(0, 0, 1, 1);
        outlineColor = [...context.getImageData(0, 0, 1, 1).data];
      }
    }

    return {
      target: {
        index: targetIndex,
        tagName: target.tagName.toLowerCase(),
        name:
          target.getAttribute("aria-label") ??
          target.textContent?.replace(/\s+/g, " ").trim() ??
          "",
        queueActionId: target.getAttribute("data-queue-action-id"),
        queueRowId:
          target.closest("[data-queue-row]")?.getAttribute("data-queue-row") ?? null
      },
      visible:
        target.checkVisibility?.({ checkOpacity: true, checkVisibilityCSS: true }) !== false,
      outlineStyle: computed.outlineStyle,
      outlineWidth: Number.parseFloat(computed.outlineWidth),
      outlineColor,
      boxShadow: computed.boxShadow
    };
  });
}
