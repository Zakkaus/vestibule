import { expect, test, type Page, type Route } from "@playwright/test";

const selectedGroupID = "-1001163306055";
const actorID = "741928306";

type SettingSource = "factory default" | "user file" | "chat override";
type QuestionLanguage = "zh" | "zh-Hant" | "en";

type SourcedSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

type Question = Readonly<{
  q: string;
  options: readonly string[];
  answer: number;
}>;

type ShortQuestion = Readonly<{
  q: string;
  answers: readonly string[];
}>;

type SettingsResponse = Readonly<{
  revision: number;
  questions: SourcedSetting<readonly Question[]>;
  fallback_questions: SourcedSetting<readonly ShortQuestion[]>;
  fallback_builtin: SourcedSetting<boolean>;
  lang: SourcedSetting<QuestionLanguage>;
}>;

type SettingsReadHandler = (route: Route, requestNumber: number) => Promise<void>;
type SettingsPatchHandler = (route: Route) => Promise<void>;

const firstQuestion: Question = {
  q: "Which package manager belongs to Gentoo?",
  options: ["Portage", "apt"],
  answer: 0
};

const secondQuestion: Question = {
  q: "Which command shows the kernel release?",
  options: ["uname -r", "hostname"],
  answer: 0
};

function sourced<T>(value: T, source: SettingSource = "factory default"): SourcedSetting<T> {
  return { value, source };
}

function settingsResponse(overrides: Partial<SettingsResponse> = {}): SettingsResponse {
  return {
    revision: 7,
    questions: sourced([firstQuestion], "user file"),
    fallback_questions: sourced([]),
    fallback_builtin: sourced(true),
    lang: sourced("zh"),
    ...overrides
  };
}

async function fulfillJSON(route: Route, body: unknown, status = 200): Promise<void> {
  await route.fulfill({
    status,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body)
  });
}

async function mockQuestionTransport(
  page: Page,
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler
): Promise<void> {
  let reads = 0;
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = decodeURIComponent(new URL(request.url()).pathname);
    if (path === "/api/session" && request.method() === "GET") {
      await fulfillJSON(route, {
        subject: { telegram_id: actorID, role: "manager" },
        expires_at: "2026-09-01T02:00:00Z",
        csrf_token: "questions-csrf"
      });
      return;
    }
    if (path === "/api/chats" && request.method() === "GET") {
      await fulfillJSON(route, { chats: [{ id: selectedGroupID }] });
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "GET") {
      reads += 1;
      await readSettings(route, reads);
      return;
    }
    if (path === `/api/chats/${selectedGroupID}/settings` && request.method() === "PATCH") {
      await patchSettings(route);
      return;
    }
    throw new Error(`Unexpected API request: ${request.method()} ${path}`);
  });
}

async function openQuestions(
  page: Page,
  readSettings: SettingsReadHandler,
  patchSettings: SettingsPatchHandler
): Promise<void> {
  await mockQuestionTransport(page, readSettings, patchSettings);
  await page.goto(`/questions?group=${selectedGroupID}`);
  await expect(page.locator("[data-questions-page]")).toHaveAttribute(
    "data-questions-state",
    "loaded"
  );
}

test("question bank adds and edits an item, then sends only the complete questions array with CSRF", async ({
  page
}) => {
  let markPatchRequested!: () => void;
  let releasePatch!: () => void;
  const patchRequested = new Promise<void>((resolve) => {
    markPatchRequested = resolve;
  });
  const patchResponse = new Promise<void>((resolve) => {
    releasePatch = resolve;
  });
  const savedQuestions: readonly Question[] = [
    { ...firstQuestion, q: "Which package manager does Gentoo use?" },
    { q: "Pick the rolling-release distribution", options: ["Debian", "Gentoo"], answer: 1 }
  ];

  await openQuestions(
    page,
    async (route) => fulfillJSON(route, settingsResponse()),
    async (route) => {
      expect(route.request().headers()["x-csrf-token"]).toBe("questions-csrf");
      expect(route.request().postDataJSON()).toEqual({
        expected_revision: 7,
        changes: { questions: savedQuestions }
      });
      markPatchRequested();
      await patchResponse;
      await fulfillJSON(
        route,
        settingsResponse({ revision: 8, questions: sourced(savedQuestions, "chat override") })
      );
    }
  );

  await page.getByLabel("题面").first().fill("Which package manager does Gentoo use?");
  await page.getByRole("button", { name: "添加选择题" }).click();
  const added = page.locator("[data-question-item]").last();
  await added.getByLabel("题面").fill("Pick the rolling-release distribution");
  await added.getByLabel("选项 1", { exact: true }).fill("Debian");
  await added.getByLabel("选项 2", { exact: true }).fill("Gentoo");
  await added.getByRole("button", { name: "将选项 2 设为正确答案" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await patchRequested;
  await expect(page.getByRole("button", { name: "正在保存…" })).toHaveAttribute(
    "aria-disabled",
    "true"
  );

  releasePatch();
  await expect(page.locator('[data-questions-feedback="saved"]')).toContainText(
    "已保存题库设置"
  );
  await expect(page.locator("[data-questions-page]")).toHaveAttribute(
    "data-questions-state",
    "loaded"
  );
  await expect(page.getByText("来源：此群覆盖").first()).toBeVisible();
});

test("question deletion requires confirmation and language restoration writes null", async ({ page }) => {
  let requestBody: unknown;
  const remainingQuestions = [secondQuestion];
  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          revision: 9,
          questions: sourced([firstQuestion, secondQuestion], "chat override"),
          lang: sourced("en", "chat override")
        })
      ),
    async (route) => {
      requestBody = route.request().postDataJSON();
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 10,
          questions: sourced(remainingQuestions, "chat override"),
          lang: sourced("zh")
        })
      );
    }
  );

  const firstDelete = page.locator("[data-question-item]").first().getByRole("button", {
    name: "删除题目"
  });
  let dismissedMessage: string | undefined;
  page.once("dialog", async (dialog) => {
    dismissedMessage = dialog.message();
    await dialog.dismiss();
  });
  await firstDelete.click();
  expect(dismissedMessage).toContain("删除选择题 1");
  await expect(page.locator("[data-question-item]")).toHaveCount(2);

  page.once("dialog", (dialog) => dialog.accept());
  await firstDelete.click();
  await expect(page.locator("[data-question-item]")).toHaveCount(1);
  await page
    .locator('[data-question-setting="lang"]')
    .getByRole("button", { name: "恢复继承值" })
    .click();
  await page.getByRole("button", { name: "保存更改" }).click();

  await expect(page.locator('[data-questions-feedback="saved"]')).toBeVisible();
  expect(requestBody).toEqual({
    expected_revision: 9,
    changes: { questions: remainingQuestions, lang: null }
  });
  await expect(page.locator("#questions-language-select")).toHaveValue("zh");
});

test("switching custom fallback questions to built-ins clears the array override", async ({ page }) => {
  let requestBody: unknown;
  const fallbackQuestions = [{ q: "Name a Gentoo package manager", answers: ["Portage", "emerge"] }];
  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          fallback_questions: sourced(fallbackQuestions, "chat override"),
          fallback_builtin: sourced(false, "chat override")
        })
      ),
    async (route) => {
      requestBody = route.request().postDataJSON();
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 8,
          fallback_questions: sourced([], "chat override"),
          fallback_builtin: sourced(true, "chat override")
        })
      );
    }
  );

  await page.getByRole("button", { name: "内置本地化题目" }).click();
  await page.getByRole("button", { name: "保存更改" }).click();
  await expect(page.locator('[data-questions-feedback="saved"]')).toBeVisible();
  expect(requestBody).toEqual({
    expected_revision: 7,
    changes: { fallback_builtin: true, fallback_questions: null }
  });
  await expect(page.getByText("机器人会按上面的答题语言")).toBeVisible();
});

test("question validation rejects an empty prompt and a blank custom fallback answer before PATCH", async ({
  page
}) => {
  let patchRequests = 0;
  await openQuestions(
    page,
    async (route) =>
      fulfillJSON(
        route,
        settingsResponse({
          fallback_questions: sourced([{ q: "Package manager?", answers: ["Portage"] }]),
          fallback_builtin: sourced(false)
        })
      ),
    async () => {
      patchRequests += 1;
      throw new Error("Client validation must prevent this PATCH request");
    }
  );

  await page.getByLabel("题面").first().fill("");
  await page.getByLabel("答案 1", { exact: true }).fill("   ");
  await page.getByRole("button", { name: "保存更改" }).click();

  await expect(page.getByLabel("题面").first()).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByText("请输入题面。")).toBeVisible();
  await expect(page.getByText("可接受答案不能留空。")).toBeVisible();
  expect(patchRequests).toBe(0);
});

test("question conflict reloads the newer arrays and explains why different-item edits collide", async ({
  page
}) => {
  let reads = 0;
  let markLatestSettingsSettled!: () => void;
  const latestSettingsSettled = new Promise<void>((resolve) => {
    markLatestSettingsSettled = resolve;
  });
  const latestQuestions = [firstQuestion, { ...secondQuestion, q: "Changed by another administrator" }];

  await openQuestions(
    page,
    async (route, requestNumber) => {
      reads = requestNumber;
      if (requestNumber === 1) {
        await fulfillJSON(
          route,
          settingsResponse({ revision: 12, questions: sourced([firstQuestion, secondQuestion]) })
        );
        return;
      }
      await fulfillJSON(
        route,
        settingsResponse({
          revision: 13,
          questions: sourced(latestQuestions, "chat override")
        })
      );
      markLatestSettingsSettled();
    },
    async (route) => {
      expect(route.request().postDataJSON()).toEqual({
        expected_revision: 12,
        changes: {
          questions: [
            { ...firstQuestion, q: "Edited in this draft" },
            secondQuestion
          ]
        }
      });
      await fulfillJSON(route, { error: { code: "settings_conflict" } }, 409);
    }
  );

  await page.getByLabel("题面").first().fill("Edited in this draft");
  await page.getByRole("button", { name: "保存更改" }).click();
  await latestSettingsSettled;
  await expect(page.locator("[data-questions-page]")).toHaveAttribute(
    "data-questions-state",
    "loaded"
  );
  await expect(page.getByLabel("题面").first()).toHaveValue(firstQuestion.q);
  await expect(page.getByLabel("题面").nth(1)).toHaveValue("Changed by another administrator");
  const conflict = page.locator('[data-questions-feedback="conflict"]');
  await expect(conflict).toContainText("另一位管理员");
  await expect(conflict).toContainText("不同题目");
  await expect(conflict).toContainText("版本冲突");
  expect(reads).toBe(2);
});
