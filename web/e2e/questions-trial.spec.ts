import { expect, test, type APIRequestContext, type Page, type Route } from "@playwright/test";

import { selectAppOption } from "./app-select";

const portOffset = Number.parseInt(process.env.PLAYWRIGHT_PORT_OFFSET ?? "0", 10);
const trialAddress = `http://127.0.0.1:${4175 + portOffset}`;
const mockGroupAID = "-1009000000011";
const mockGroupBID = "-1009000000012";

type TrialSession = Readonly<{
  cookies: readonly Readonly<{ name: string; value: string }>[];
  chat_id: string;
}>;

type MockTrialHandler = (route: Route, body: Record<string, unknown>) => Promise<void>;

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function forwardTrialAPI(
  page: Page,
  rewriteTrialRequest?: (body: Record<string, unknown>) => Record<string, unknown>
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const source = new URL(request.url());
    const target = `${trialAddress}${source.pathname}${source.search}`;
    if (
      rewriteTrialRequest &&
      request.method() === "POST" &&
      source.pathname.endsWith("/rules/test")
    ) {
      const body = JSON.parse(request.postData() ?? "{}") as Record<string, unknown>;
      await route.continue({ url: target, postData: JSON.stringify(rewriteTrialRequest(body)) });
      return;
    }
    await route.continue({ url: target });
  });
}

async function openRealTrial(
  page: Page,
  request: APIRequestContext,
  rewriteTrialRequest?: (body: Record<string, unknown>) => Record<string, unknown>
): Promise<string> {
  const response = await request.get(`${trialAddress}/__trial/session`);
  expect(response.ok()).toBe(true);
  const session = (await response.json()) as TrialSession;
  await page.context().addCookies(
    session.cookies.map((cookie) => ({ name: cookie.name, value: cookie.value, url: `${trialAddress}/` }))
  );
  await forwardTrialAPI(page, rewriteTrialRequest);
  await page.goto(`/questions?group=${session.chat_id}`);
  await expect(page.locator("[data-questions-page]")).toHaveAttribute("data-questions-state", "loaded");
  await expect(trialRegion(page)).toBeVisible();
  return session.chat_id;
}

function mockSettings(question: string, secondQuestion: string, revision = 7) {
  return {
    revision,
    questions: {
      value: [
        { q: question, options: ["triangle", "square"], answer: 0 },
        { q: secondQuestion, options: ["circle", "hexagon"], answer: 0 }
      ],
      source: "chat override"
    },
    fallback_questions: {
      value: [{ q: "Name a package manager", answers: ["Portage", "emerge"] }],
      source: "chat override"
    },
    fallback_builtin: { value: false, source: "chat override" },
    lang: { value: "zh", source: "chat override" }
  };
}

async function installMockTransport(
  page: Page,
  trial: MockTrialHandler = (route) => fulfillJSON(route, { correct: true })
): Promise<void> {
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);
    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: "741928306", role: "manager" },
        expires_at: "2026-09-01T02:00:00Z",
        csrf_token: "questions-trial-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, {
        chats: [
          { id: mockGroupAID, title: "Trial group A" },
          { id: mockGroupBID, title: "Trial group B" }
        ]
      });
      return;
    }
    if (path === `/api/chats/${mockGroupAID}/settings` && request.method() === "GET") {
      await fulfillJSON(route, mockSettings("Group A triangle?", "Group A second?"));
      return;
    }
    if (path === `/api/chats/${mockGroupBID}/settings` && request.method() === "GET") {
      await fulfillJSON(route, mockSettings("Group B triangle?", "Group B second?", 8));
      return;
    }
    if (path.endsWith("/rules/test") && request.method() === "POST") {
      const body = JSON.parse(request.postData() ?? "{}") as Record<string, unknown>;
      await trial(route, body);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openMockQuestions(page: Page, trial?: MockTrialHandler): Promise<void> {
  await installMockTransport(page, trial);
  await page.goto(`/questions?group=${mockGroupAID}`);
  await expect(page.locator("[data-questions-page]")).toHaveAttribute("data-questions-state", "loaded");
  await expect(trialRegion(page)).toBeVisible();
}

function trialRegion(page: Page) {
  return page.getByRole("region", { name: "试答已保存题目" });
}

async function submitTrial(page: Page): Promise<void> {
  await page.getByRole("button", { name: "提交试答" }).click();
}

test("saved quiz trial reports a correct answer through the real API", async ({ page, request }) => {
  await openRealTrial(page, request);
  await trialRegion(page).getByRole("button", { name: "triangle", exact: true }).click();
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答正确");
});
test("saved fallback trial uses the server answer matcher", async ({ page, request }) => {
  await openRealTrial(page, request);
  await selectAppOption(page.getByRole("button", { name: "题库", exact: true }), "fallback_questions");
  await page.getByLabel("输入答案").fill("Portage");
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答正确");
});

test("saved quiz trial reports a wrong answer through the real API", async ({ page, request }) => {
  await openRealTrial(page, request);
  await trialRegion(page).getByRole("button", { name: "square", exact: true }).click();
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答错误");
});

test("saved quiz trial presents a real missing-question error", async ({ page, request }) => {
  await openRealTrial(page, request, (body) => ({ ...body, question_index: 999 }));
  await trialRegion(page).getByRole("button", { name: "triangle", exact: true }).click();
  const trialResponse = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname.endsWith("/rules/test") && response.request().method() === "POST";
  });
  await submitTrial(page);
  const response = await trialResponse;
  expect(response.status()).toBe(404);
  const payload = (await response.json()) as { error?: { code?: unknown } };
  expect(payload.error?.code).toBe("rule_not_found");
  await expect(trialRegion(page).getByRole("alert")).toContainText("找不到");
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
});

test("a saved question change refuses the old trial revision", async ({ page, request }) => {
  const chatID = await openRealTrial(page, request);
  const patchStatus = await page.evaluate(async (group) => {
    const session = await (await fetch("/api/session")).json();
    const settings = await (await fetch(`/api/chats/${group}/settings`)).json();
    const response = await fetch(`/api/chats/${group}/settings`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", "X-CSRF-Token": session.csrf_token },
      body: JSON.stringify({
        expected_revision: settings.revision,
        changes: { questions: settings.questions.value.map((question: { q: string }) => ({
          ...question, q: `${question.q} revised`
        })) }
      })
    });
    return response.status;
  }, chatID);
  expect(patchStatus).toBe(200);
  const response = page.waitForResponse((item) => item.url().endsWith("/rules/test"));
  await submitTrial(page);
  expect((await response).status()).toBe(409);
  await expect(trialRegion(page).getByRole("alert")).toBeVisible();
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
  await page.getByRole("button", { name: "重新读取已保存题目" }).click();
  await expect(trialRegion(page).getByRole("paragraph").filter({ hasText: /revised/ })).toBeVisible();
  await expect(trialRegion(page).getByRole("alert")).toHaveCount(0);
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答正确");
});

test("saving an edited question clears its previous trial result", async ({ page, request }) => {
  await openRealTrial(page, request);
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答正确");
  await page.getByLabel("题面").first().fill("Edited saved quiz prompt");
  await page.getByRole("button", { name: "保存更改", exact: true }).click();
  await expect(trialRegion(page).getByText("Edited saved quiz prompt", { exact: true })).toBeVisible();
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
});

test("trial reads saved settings while an editor draft changes", async ({ page }) => {
  await openMockQuestions(page);
  await expect(trialRegion(page).getByText("Group A triangle?", { exact: true })).toBeVisible();
  await page.getByLabel("题面").first().fill("Unsaved draft prompt");
  await expect(trialRegion(page).getByText("Group A triangle?", { exact: true })).toBeVisible();
});

test("trial result clears when the answer or question changes", async ({ page }) => {
  await openMockQuestions(page);
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答正确");
  await trialRegion(page).getByRole("button", { name: "square", exact: true }).click();
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答正确");
  await selectAppOption(page.getByRole("button", { name: "题目", exact: true }), "1");
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答正确");
  await selectAppOption(page.getByRole("button", { name: "题库", exact: true }), "fallback_questions");
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
  await page.getByLabel("输入答案").fill("Portage");
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toContainText("回答正确");
  await page.getByLabel("输入答案").fill("emerge");
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
});

test("trial result clears when the selected group changes", async ({ page }) => {
  await openMockQuestions(page);
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("status")).toHaveCount(1);
  await selectAppOption(page.getByRole("button", { name: "当前群" }), mockGroupBID);
  await expect(page).toHaveURL(new RegExp(`/questions\\?group=${mockGroupBID}$`));
  await expect(trialRegion(page)).toBeVisible();
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
  await expect(trialRegion(page).getByText("Group B triangle?", { exact: true })).toBeVisible();
});

test("a delayed trial result cannot replace a changed answer", async ({ page }) => {
  let releaseResponse!: () => void;
  let markStarted!: () => void;
  let markSettled!: () => void;
  let requestCount = 0;
  const responseGate = new Promise<void>((resolve) => { releaseResponse = resolve; });
  const requestStarted = new Promise<void>((resolve) => { markStarted = resolve; });
  const responseSettled = new Promise<void>((resolve) => { markSettled = resolve; });
  await openMockQuestions(page, async (route) => {
    requestCount += 1;
    if (requestCount === 1) {
      markStarted();
      await responseGate;
      await fulfillJSON(route, { correct: true });
      markSettled();
      return;
    }
    await fulfillJSON(route, { correct: false });
  });
  await submitTrial(page);
  await requestStarted;
  await trialRegion(page).getByRole("button", { name: "square", exact: true }).click();
  const latestResponse = page.waitForResponse((response) => {
    const url = new URL(response.url());
    return url.pathname.endsWith("/rules/test") && response.request().method() === "POST";
  });
  await submitTrial(page);
  await latestResponse;
  await expect(trialRegion(page).getByRole("status")).toContainText("回答错误");
  const oldResponse = page.waitForResponse((response) =>
    response.url().endsWith("/rules/test") && response.request().postDataJSON().choice === 0
  );
  releaseResponse();
  await responseSettled;
  await (await oldResponse).finished();
  await page.evaluate(() => new Promise<void>((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  ));
  await expect(trialRegion(page).getByRole("status")).toContainText("回答错误");
});

test("a malformed trial result is rejected instead of shown as a judgment", async ({ page }) => {
  await openMockQuestions(page, (route) => fulfillJSON(route, { correct: "true" }));
  await submitTrial(page);
  await expect(trialRegion(page).getByRole("alert")).toContainText("无法识别");
  await expect(trialRegion(page).getByRole("status")).toHaveCount(0);
});
