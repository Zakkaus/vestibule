import { expect, test } from "@playwright/test";

// The group declares the gap between its children. The label also carried a
// bottom margin, so the label sat one and a half times further from the first
// item than the items sat from each other, and the column read as unevenly
// spaced. One relationship, one declaration.
test("the sidebar spaces its label and its items by one rule", async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const json = (body: unknown) =>
      route.fulfill({ headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (path === "/api/session") {
      return json({
        subject: { telegram_id: "741928306", role: "manager" },
        expires_at: "2027-01-01T00:00:00Z",
        csrf_token: "test-csrf"
      });
    }
    if (path === "/api/chats") {
      return json({ chats: [{ id: "-1009123456789" }] });
    }
    if (path === "/api/instance") {
      return json({ bot_username: "example_bot" });
    }
    return json({});
  });

  await page.goto("/queue?group=-1009123456789");
  await expect(page.locator(".nav-label").first()).toBeVisible();

  const gaps = await page.evaluate(() => {
    const items = Array.from(document.querySelectorAll(".nav-item")).slice(0, 4);
    const label = document.querySelector(".nav-label");
    if (!label || items.length < 4) {
      return null;
    }
    const box = (element: Element) => element.getBoundingClientRect();
    return {
      labelToFirst: Math.round(box(items[0]).top - box(label).bottom),
      between: items.slice(1).map((item, index) => Math.round(box(item).top - box(items[index]).bottom))
    };
  });

  if (!gaps) {
    throw new Error("the sidebar did not render a label and four items");
  }
  for (const gap of gaps.between) {
    expect(gap).toBe(gaps.between[0]);
  }
  expect(gaps.labelToFirst).toBe(gaps.between[0]);
});
