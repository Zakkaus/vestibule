import { expect, test, type Page } from "@playwright/test";
import { mockSpectrumTransport, selectedGroupID } from "./spectrum-fixtures";
import { selectConsolePreference } from "./app-select";

async function openTemplate(page: Page, locale: string) {
  await page.addInitScript(locale => localStorage.setItem("verify-console-locale", locale), locale);
  await mockSpectrumTransport(page);
  await page.goto(`/verification?group=${selectedGroupID}`);
  await expect(page.locator('[data-verification-state="loaded"]')).toBeVisible();
  await page.evaluate(() => document.fonts.ready);
}

async function saveTemplate(page: Page) {
  const values = { delivery_mode: "group", verify_mode: "mixed", timeout_seconds: 360,
    verify_max_fails: 3, verify_retry_seconds: 180, ban_seconds: 360, mute_seconds: 3600, verify_invited: true };
  await page.route('**/api/chats/*/settings', async route => {
    if (route.request().method() !== 'PATCH') return route.fallback();
    await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({revision:8,
      ...Object.fromEntries(Object.entries(values).map(([key,value]) => [key,{value,source:'factory default'}]))})});
  });
  await page.locator('[data-verification-savebar] button[type="submit"]').click();
  await expect(page.locator('[data-console-toasts]')).toBeVisible();
  await expect(page.locator('[data-verification-savebar] button')).toHaveCount(1);
}

async function textProblems(page: Page) {
  return page.evaluate(() => {
    const problems: string[] = [];
    const roots = document.querySelectorAll<HTMLElement>('[data-verification-page] h1, [data-verification-page] h2, [data-verification-page] p, [data-verification-setting], [data-verification-page] label, [data-verification-page] button, [data-hub-pages] a, [data-hub-bar] a, [role="menuitem"], [role="menuitemradio"], [role="option"]');
    for (const element of roots) {
      if (!element.checkVisibility()) continue;
      const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
      const lines = new Set<number>();
      let node: Node | null;
      while ((node = walker.nextNode())) {
        const text = node.textContent ?? "";
        for (const match of text.matchAll(/\S+/g)) {
          const range = document.createRange();
          range.setStart(node, match.index!); range.setEnd(node, match.index! + match[0].length);
          const rects = [...range.getClientRects()].filter(rect => rect.width > 0);
          const rows = new Set(rects.map(rect => Math.round(rect.top)));
          if (rows.size > 1 && /[A-Za-z\u0400-\u04ff]/.test(match[0])) problems.push(`split word: ${match[0]}`);
          for (const rect of rects) {
            lines.add(Math.round(rect.top));
            const bounds = element.getBoundingClientRect();
            if (rect.left < bounds.left - 1 || rect.right > bounds.right + 1) problems.push(`clipped: ${text}`);
          }
        }
      }
      if (element.tagName === "BUTTON" && lines.size > 1) problems.push(`wrapped button: ${element.textContent}`);
      if (getComputedStyle(element).textOverflow === "ellipsis" && element.scrollWidth > element.clientWidth) problems.push(`truncated: ${element.textContent}`);
    }
    return problems;
  });
}

for (const locale of ["zh-CN", "ru"]) for (const width of [390, 768, 1280]) for (const touch of [false, true]) {
  test.describe(`${locale} ${width}px ${touch ? "touch" : "fine"} template`, () => {
    test.use({ viewport: { width, height: width === 390 ? 844 : 1024 }, hasTouch: touch });
    test("shell, cards, grid, controls and dock follow the reference geometry", async ({ page }) => {
      await openTemplate(page, locale);
      const geometry = await page.evaluate(() => {
        const box = (selector: string) => document.querySelector<HTMLElement>(selector)!;
        const main = box('.console-content'), card = box('[aria-labelledby="verification-timing-title"]');
        const grid = box('[aria-labelledby="verification-timing-title"] [data-settings-field-grid]');
        const cards = [...document.querySelectorAll<HTMLElement>('[data-verification-section]')];
        const field = box('[data-verification-setting="timeout_seconds"]');
        const css = getComputedStyle(main), cardCSS = getComputedStyle(card), gridCSS = getComputedStyle(grid);
        return { rootWidth: document.documentElement.clientWidth, shellWidth: box('.console-shell').clientWidth,
          top: box('[data-console-header]').getBoundingClientRect().height, sidebar: box('.console-sidebar').getBoundingClientRect().width,
          mainX: main.getBoundingClientRect().x, mainWidth: main.clientWidth, gutter: parseFloat(css.paddingLeft), topPadding: parseFloat(css.paddingTop),
          contentWidth: box('.console-inner').clientWidth, cardPadding: parseFloat(cardCSS.paddingLeft), border: parseFloat(cardCSS.borderLeftWidth), radius: parseFloat(cardCSS.borderTopLeftRadius),
          columns: gridCSS.gridTemplateColumns.split(' ').map(Number.parseFloat), gridWidth: grid.clientWidth, gridGap: parseFloat(gridCSS.gap), stackGap: parseFloat(getComputedStyle(field).gap),
          sectionGap: cards[1]!.getBoundingClientRect().top - cards[0]!.getBoundingClientRect().bottom,
          sectionBodyGap: parseFloat(cardCSS.gap), controlWidths: [...grid.querySelectorAll('input')].map(el => [el.parentElement!.getBoundingClientRect().width, el.closest('[data-verification-setting]')!.getBoundingClientRect().width]),
          trackWidths: cards.slice(0,2).map(el => el.querySelector('button')!.getBoundingClientRect().width),
          heights: [...document.querySelectorAll<HTMLElement>('[data-verification-form] button, [data-console-header] button, [data-verification-form] input:not([type="checkbox"])')].filter(el => el.checkVisibility()).map(el => (el.tagName === 'INPUT' ? el.parentElement! : el).getBoundingClientRect().height),
          bottomPadding: parseFloat(css.paddingBottom), overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
          overflowY: css.overflowY, footerPosition: getComputedStyle(box('[data-verification-savebar]')).position };
      });
      expect(geometry.top).toBeCloseTo(64, 0);
      expect(geometry.shellWidth).toBeLessThanOrEqual(1600);
      expect(geometry.gutter).toBe(width === 390 ? 16 : 40);
      expect(geometry.topPadding).toBe(width === 390 ? 20 : 32);
      expect(geometry.contentWidth).toBe(Math.min(1120, geometry.rootWidth - (width >= 1024 ? 256 : 0) - 2 * geometry.gutter));
      if (width === 1280) { expect(geometry.sidebar).toBe(240); expect(geometry.mainX + geometry.mainWidth).toBe(geometry.rootWidth - 16); }
      expect(geometry.cardPadding).toBe(16); expect(geometry.border).toBe(1); expect(geometry.radius).toBe(16);
      expect(geometry.sectionGap).toBeCloseTo(24, 0); expect(geometry.sectionBodyGap).toBe(12);
      expect(geometry.gridGap).toBe(12); expect(geometry.stackGap).toBe(4);
      const columns = Math.floor((geometry.gridWidth + 12) / 252);
      expect(geometry.columns).toHaveLength(columns);
      for (const column of geometry.columns) expect(Math.abs(column - (geometry.gridWidth - 12 * (columns - 1)) / columns)).toBeLessThanOrEqual(1);
      for (const [control, cell] of geometry.controlWidths) expect(Math.abs(control! - cell!)).toBeLessThanOrEqual(1);
      for (const track of geometry.trackWidths) expect(Math.abs(track - geometry.columns[0]!)).toBeLessThanOrEqual(1);
      for (const height of geometry.heights) expect(Math.abs(height - (touch ? 40 : 32))).toBeLessThanOrEqual(1);
      expect(geometry.bottomPadding).toBe(width === 390 ? 104 : width < 1024 ? 112 : 48);
      expect(geometry.overflow).toBeLessThanOrEqual(1); expect(geometry.overflowY).toBe("visible"); expect(geometry.footerPosition).toBe("static");
      expect(await textProblems(page)).toEqual([]);
      await page.locator('#verification-timeout-seconds').fill('360');
      const footer = page.locator('[data-verification-savebar]');
      await footer.scrollIntoViewIfNeeded();
      await expect(footer.getByRole('button')).toHaveCount(2);
      expect(await textProblems(page)).toEqual([]);
      await page.evaluate(() => document.body.style.setProperty('--console-safe-bottom', '34px'));
      if (width < 1024) {
        expect(await page.locator('[data-hub-bar]').evaluate(el => el.getBoundingClientRect().height)).toBe(98);
        const clearance = await footer.evaluate(el => window.innerHeight - el.getBoundingClientRect().bottom);
        await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
        expect(await footer.evaluate(el => window.innerHeight - el.getBoundingClientRect().bottom)).toBeGreaterThanOrEqual((width === 390 ? 138 : 146) - 1);
        expect(clearance).toBeGreaterThanOrEqual(0);
      }
      await saveTemplate(page);
      expect(await page.locator('[data-console-toasts]').evaluate(el => parseFloat(getComputedStyle(el).bottom))).toBe(width < 1024 ? 114 : 50);
      await page.evaluate(() => document.body.style.setProperty('--console-safe-top', '20px'));
      expect(await page.locator('[data-console-header]').evaluate(el => el.getBoundingClientRect().height)).toBe(84);
      await selectConsolePreference(page, 'locale', locale === 'ru' ? 'zh-CN' : 'ru');
      await expect(page.locator('#verification-timeout-seconds')).toHaveValue('360');
      expect(await textProblems(page)).toEqual([]);
    });
  });
}

test("hubs remember the last committed page, including Back and canceled dirty exits", async ({ page }) => {
  await page.setViewportSize({width:390,height:844});
  await openTemplate(page, 'en');
  await page.locator('[data-hub-pages] [data-navigation-item="/questions"]').click();
  await page.locator('[data-hub="daily"]').click();
  await page.locator('[data-hub="verification"]').click();
  await expect(page).toHaveURL(url => url.pathname === '/questions');
  await page.goBack();
  await expect(page).toHaveURL(url => url.pathname === '/home');
  await page.locator('[data-hub="verification"]').click();
  await page.locator('[data-hub-pages] [data-navigation-item="/verification"]').click();
  await page.locator('#verification-timeout-seconds').fill('360');
  await page.locator('[data-hub="daily"]').click();
  await page.getByRole('alertdialog').getByRole('button',{name:'Cancel',exact:true}).click();
  await expect(page).toHaveURL(url => url.pathname === '/verification');
  await expect(page.locator('#verification-timeout-seconds')).toHaveValue('360');
});

test("the shell caps its desktop columns and releases the toolbar in landscape", async ({ page }) => {
  await page.setViewportSize({width:1920,height:1080});
  await openTemplate(page, 'en');
  expect(await page.locator('.console-shell').evaluate(el => el.clientWidth)).toBe(1600);
  expect(await page.locator('.console-inner').evaluate(el => el.clientWidth)).toBe(1120);
  expect(await page.locator('[aria-labelledby="verification-timing-title"] [data-settings-field-grid]').evaluate(el => getComputedStyle(el).gridTemplateColumns.split(' ').length)).toBe(4);
  await page.setViewportSize({width:844,height:390});
  await expect(page.locator('[data-console-header]')).toHaveCSS('position','static');
  await page.locator('[data-verification-savebar]').scrollIntoViewIfNeeded();
  await expect(page.locator('[data-verification-savebar]')).toBeInViewport({ratio:1});
});

for (const locale of ['zh-CN','ru']) test.describe(`phone template states in ${locale}`, () => {
  test.use({viewport:{width:390,height:844},hasTouch:true});
  test('retry, restore, pending, error and success retain complete labels and geometry', async ({page}) => {
    await page.addInitScript(locale => localStorage.setItem('verify-console-locale',locale),locale);
    await mockSpectrumTransport(page);
    let reads=0, writes=0;
    let release!: () => void;
    const pending = new Promise<void>(resolve => release=resolve);
    const values = {delivery_mode:'group',verify_mode:'mixed',timeout_seconds:300,verify_max_fails:3,
      verify_retry_seconds:180,ban_seconds:360,mute_seconds:3600,verify_invited:true};
    const settings = (revision:number) => ({revision,...Object.fromEntries(Object.entries(values).map(([key,value]) =>
      [key,{value,source:key === 'ban_seconds' ? 'chat override' : 'factory default'}]))});
    await page.route('**/api/chats/*/settings', async route => {
      if (route.request().method()==='GET') {
        reads++;
        return route.fulfill({status:reads===1?503:200,contentType:'application/json',
          body:JSON.stringify(reads===1?{error:{code:'temporarily_unavailable'}}:settings(7))});
      }
      writes++;
      if (writes===1) return route.fulfill({status:400,contentType:'application/json',body:JSON.stringify({error:{code:'invalid_settings'}})});
      await pending;
      values.timeout_seconds=360;
      await route.fulfill({status:200,contentType:'application/json',body:JSON.stringify(settings(8))});
    });
    await page.goto(`/verification?group=${selectedGroupID}`);
    await expect(page.locator('[data-verification-state="unavailable"]')).toBeVisible();
    const retry = page.locator('[data-verification-state="unavailable"] button');
    expect(await retry.evaluate(el=>el.getBoundingClientRect().height)).toBe(40);
    expect(await textProblems(page)).toEqual([]);
    await retry.click();
    await expect(page.locator('[data-verification-state="loaded"]')).toBeVisible();
    const restore = page.locator('[data-verification-setting="ban_seconds"] button');
    expect(await restore.evaluate(el=>el.getBoundingClientRect().height)).toBe(40);
    await expect(restore).toHaveAccessibleName(/\S+/);
    await restore.click();
    const footer = page.locator('[data-verification-savebar]');
    await expect(footer.locator('button')).toHaveCount(2);
    await footer.locator('button:not([type="submit"])').click();
    await expect(page.locator('#verification-ban-seconds')).toHaveValue('360');
    expect(writes).toBe(0);
    await page.locator('#verification-timeout-seconds').fill('360');
    const save = footer.getByRole('button').first();
    await save.click();
    const error = page.locator('[data-verification-page] [data-verification-feedback]');
    await expect(error).toHaveAttribute('data-feedback-level','negative');
    expect(await error.evaluate(el=>el.contains(document.activeElement))).toBe(true);
    const positions = await page.evaluate(() => ({error:document.querySelector('[data-verification-feedback]')!.getBoundingClientRect().bottom,
      card:document.querySelector('[data-verification-section]')!.getBoundingClientRect().top}));
    expect(positions.card-positions.error).toBeCloseTo(24,0);
    await expect(page.locator('#verification-timeout-seconds')).toHaveValue('360');
    await save.click();
    await expect.poll(()=>writes).toBe(2);
    await expect(save).toBeDisabled();
    await expect(footer.getByRole('button').nth(1)).toBeDisabled();
    expect(await save.evaluate(el=>el.getBoundingClientRect().height)).toBe(40);
    expect(await textProblems(page)).toEqual([]);
    release();
    await expect(page.locator('[data-console-toasts]')).toBeVisible();
    await expect(error).toHaveCount(0);
    await expect(footer.locator('button')).toHaveCount(1);
    await expect(save).toBeDisabled();
  });
});
