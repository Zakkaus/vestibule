import { expect, test, type Page, type Route } from "@playwright/test";

import { installSpectrumClock, mockSpectrumTransport } from "./spectrum-fixtures";

const groupID = "-1009000010001";

const sourced = <Value,>(value: Value) => ({ value, source: "factory default" as const });

const settings = {
  revision: 7,
  questions: sourced([
    {
      q: "Which package manager belongs to Gentoo?",
      options: ["Portage", "apt"],
      answer: 0
    }
  ]),
  fallback_questions: sourced([
    { q: "Name a Gentoo package manager", answers: ["Portage", "emerge"] }
  ]),
  fallback_builtin: sourced(false),
  lang: sourced("zh"),
  name_spoiler: sourced(true),
  lookup_auto_delete_enabled: sourced(true),
  lookup_ttl_seconds: sourced(180),
  rich_messages: sourced(false)
};

const rules = [
  {
    id: "auto-a",
    collection: "auto_reply",
    ordinal: 0,
    enabled: true,
    definition: { match: ["matrix"], reply: { text: "Bridge address" } }
  }
];

type Screen = Readonly<{
  name: string;
  url: string;
  ready: string;
  state?: readonly [name: string, value: string];
}>;

const screens: readonly Screen[] = [
  {
    name: "question editor",
    url: `/questions?group=${groupID}`,
    ready: "[data-questions-page]",
    state: ["data-questions-state", "loaded"]
  },
  {
    name: "statistics filters",
    url: `/stats?group=${groupID}`,
    ready: "[data-stats-page]",
    state: ["data-stats-state", "loaded"]
  },
  {
    name: "message rules",
    url: `/messages?group=${groupID}`,
    ready: "[data-messages-settings-form]"
  }
];

async function fulfillJSON(route: Route, body: unknown): Promise<void> {
  await route.fulfill({
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

function stats(url: URL) {
  const from = url.searchParams.get("from") ?? "2026-09-01";
  const to = url.searchParams.get("to") ?? "2026-09-08";
  const timezone = url.searchParams.get("timezone") ?? "UTC";
  const outcome = {
    challenges: 10,
    approved: 5,
    declined: 2,
    banned: 1,
    expired: 2,
    pass_rate: 0.5
  };
  return {
    range: { from, to, timezone },
    summary: outcome,
    trend: [{ date: from, ...outcome }],
    interceptions: [{ kind: "kernel", count: 3 }]
  };
}

async function mockControlScreens(page: Page): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = decodeURIComponent(url.pathname);

    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: "741928306", role: "manager" },
        expires_at: "2026-09-02T12:00:00Z",
        is_owner: false,
        csrf_token: "control-size-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: groupID, title: "Gentoo-zh Community", owner: null, administrators: [], administrators_status: "unavailable" }] });
      return;
    }
    if (path === `/api/chats/${groupID}/settings` && request.method() === "GET") {
      await fulfillJSON(route, settings);
      return;
    }
    if (path === `/api/chats/${groupID}/stats` && request.method() === "GET") {
      await fulfillJSON(route, stats(url));
      return;
    }
    if (path === `/api/chats/${groupID}/rules` && request.method() === "GET") {
      await fulfillJSON(route, { items: rules });
      return;
    }

    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

type Control = Readonly<{
  label: string;
  height: string;
}>;

type ParallelControlGroup = Readonly<{
  container: string;
  controls: readonly Control[];
}>;

async function parallelControlGroups(page: Page): Promise<readonly ParallelControlGroup[]> {
  return page.evaluate(() => {
    const controlSelector = [
      '[data-slot="button"]',
      '[data-slot="input"]',
      '[data-slot="textarea"]',
      '[data-slot="switch"]',
      'button[data-console-control]',
      'a[data-console-control]',
      '[data-console-control] > button',
      '[data-console-control] input:not([type="hidden"])'
    ].join(", ");
    const rowCoordinate = (rect: DOMRect, alignment: string) => {
      switch (alignment) {
        case "end":
        case "flex-end":
          return rect.bottom;
        case "start":
        case "flex-start":
          return rect.top;
        default:
          return rect.top + rect.height / 2;
      }
    };
    const describe = (element: HTMLElement, index: number) => {
      const slot = element.getAttribute("data-slot") ?? "control";
      const size = element.getAttribute("data-size") ?? "default";
      const name =
        element.getAttribute("aria-label") ?? element.textContent?.replace(/\s+/g, " ").trim() ?? "";
      return `${index + 1}. [data-slot="${slot}"][data-size="${size}"]${name ? ` ${name}` : ""}`;
    };
    const containerName = (element: HTMLElement) => {
      const dataName = element.getAttributeNames().find((name) => name.startsWith("data-"));
      return `${element.tagName.toLowerCase()}${dataName ? `[${dataName}]` : ""}`;
    };

    return [...document.querySelectorAll<HTMLElement>("*")].flatMap((container) => {
      const display = getComputedStyle(container).display;
      if (!["flex", "grid", "inline-flex", "inline-grid"].includes(display)) {
        return [];
      }

      const candidates = [...container.children].flatMap((child) => {
        if (
          !(child instanceof HTMLElement) ||
          child.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }) === false
        ) {
          return [];
        }
        if (child.matches(controlSelector)) {
          return [{ branch: child, control: child }];
        }
        const nested = [...child.querySelectorAll<HTMLElement>(controlSelector)].filter(
          (element) =>
            element.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }) !== false
        );
        const control = nested.at(0);
        return control && nested.length === 1 ? [{ branch: child, control }] : [];
      });
      if (candidates.length < 2) {
        return [];
      }

      const alignment = getComputedStyle(container).alignItems;
      const rows: { coordinate: number; controls: typeof candidates }[] = [];
      for (const candidate of candidates) {
        const coordinate = rowCoordinate(candidate.branch.getBoundingClientRect(), alignment);
        const row = rows.find((current) => Math.abs(current.coordinate - coordinate) <= 1);
        if (row) {
          row.controls.push(candidate);
        } else {
          rows.push({ coordinate, controls: [candidate] });
        }
      }

      return rows
        .filter((row) => row.controls.length >= 2)
        .map((row) => ({
          container: containerName(container),
          controls: row.controls.map(({ control }, index) => ({
            label: describe(control, index),
            height: getComputedStyle(control).height
          }))
        }));
    });
  });
}

test("parallel controls keep a shared computed height across loaded console screens", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await mockControlScreens(page);

  for (const screen of screens) {
    await test.step(screen.name, async () => {
      await page.goto(screen.url);
      const root = page.locator(screen.ready);
      await expect(root).toBeVisible();
      if (screen.state) {
        await expect(root).toHaveAttribute(...screen.state);
      }
      await page.evaluate(async () => {
        await document.fonts.ready;
        await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      });

      const groups = await parallelControlGroups(page);
      expect(groups, `${screen.name} must expose parallel control groups`).not.toEqual([]);
      for (const group of groups) {
        expect(
          new Set(group.controls.map((control) => control.height)).size,
          `${screen.name} ${group.container}: ${group.controls
            .map((control) => `${control.label}=${control.height}`)
            .join(", ")}`
        ).toBe(1);
      }
      if (screen.name === "question editor") {
        const editorHeights = await page
          .locator(
            [
              '[data-question-item] [data-question-item-heading] [data-slot="button"]',
              '[data-question-item] [data-question-option-row] [data-slot="button"]',
              '[data-question-item] [data-question-option-row] [data-slot="input"]',
              '[data-fallback-item] [data-fallback-answer-row] [data-slot="button"]',
              '[data-fallback-item] [data-fallback-answer-row] [data-slot="input"]'
            ].join(", ")
          )
          .evaluateAll((controls) => controls.map((control) => getComputedStyle(control).height));
        expect(editorHeights, "question editor must expose action and field controls").not.toEqual([]);
        expect(new Set(editorHeights).size, "question editor action and field controls").toBe(1);
      }
    });
  }
});

for (const locale of ["en", "zh-CN", "ru"]) {
  test.describe(`picker labels in ${locale}`, () => {
    test.use({ locale });
    for (const width of [390, 1280]) {
      test(`content picker labels and card bounds at ${width}px`, async ({ page }) => {
        await page.setViewportSize({ width, height: 900 });
        await page.addInitScript(() => {
          localStorage.setItem("verify-console-theme", "system");
          localStorage.setItem("verify-console-locale", "system");
        });
        await installSpectrumClock(page);
        await mockSpectrumTransport(page);
        for (const route of ["/home", "/preferences", "/verification", "/questions", "/feeds", "/"]) {
          if (route === "/") {
            await page.route("**/api/session", (route) => route.fulfill({ status: 401, json: { error: { code: "authentication_invalid" } } }));
            await page.route("**/api/instance", (route) => route.fulfill({ json: { bot_username: "actual_instance_bot" } }));
          }
          await page.goto(route === "/preferences" || route === "/" ? route : `${route}?group=${groupID}`);
          if (route === "/") {
            await expect(page.locator("[data-entry-page]")).toHaveAttribute("data-entry-state", "no-session");
          } else if (route !== "/preferences") {
            await expect(page.locator(`[data-${route.slice(1)}-page]`)).toHaveAttribute(`data-${route.slice(1)}-state`, "loaded");
          }
          await expect(page.locator("html")).toHaveAttribute("lang", locale);
          await page.evaluate(async () => { await document.fonts.ready; });
          // Chrome retains main's capped values so translated controls cannot grow the phone header.
          const labels = await page.locator('button[aria-haspopup="listbox"]').evaluateAll((buttons) =>
            buttons.filter((button) => !button.closest('[data-console-header], [data-utility-controls][data-variant="chrome"]'))
              .flatMap((button) => [...button.parentElement!.querySelectorAll<HTMLElement>('label, [data-slot="label"], :scope > div:first-child')])
              .filter((label) => label.checkVisibility() && !label.closest('[aria-hidden="true"]'))
              .map((label) => ({ text: label.textContent, scrollWidth: label.scrollWidth, clientWidth: label.clientWidth }))
          );
          for (const label of labels) {
            expect(label.scrollWidth, `${route}: ${label.text}`).toBeLessThanOrEqual(label.clientWidth);
          }
          expect(await page.evaluate(() => document.documentElement.scrollWidth), `${route} viewport`).toBeLessThanOrEqual(width);
          if (width === 390) {
            if (route !== "/") {
              // Main c2695702996b51fb363b5879088940e7be7fa77f: 176px in en, zh-CN and ru at 390×900.
              const header = await page.locator("[data-console-header]").boundingBox();
              expect(header!.height, `${route} phone chrome`).toBeLessThanOrEqual(176);
            }
            const cards = await page.locator('[data-slot="card"]').evaluateAll((cards) =>
              cards.filter((card) => card.checkVisibility()).map((card) => {
                const bounds = card.getBoundingClientRect();
                const style = getComputedStyle(card);
                const left = bounds.left + parseFloat(style.borderLeftWidth) + parseFloat(style.paddingLeft);
                const right = bounds.right - parseFloat(style.borderRightWidth) - parseFloat(style.paddingRight);
                return {
                  right: bounds.right,
                  contentWidth: right - left,
                  pickers: [...card.querySelectorAll('button[aria-haspopup="listbox"]')].map((trigger) => ({
                    text: trigger.textContent,
                    width: trigger.getBoundingClientRect().width
                  }))
                };
              })
            );
            for (const card of cards) {
              expect(card.right, `${route} card gutter`).toBeLessThanOrEqual(width - 16);
              for (const picker of card.pickers) {
                expect(picker.width, `${route}: ${picker.text} content box`).toBeLessThanOrEqual(card.contentWidth);
              }
            }
          }
        }
      });
    }
  });
}

