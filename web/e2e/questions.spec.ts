import { expect, test, type Locator, type Page, type Route } from "@playwright/test";

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

type Deferred = Readonly<{
  promise: Promise<void>;
  resolve: () => void;
}>;

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

function busyControlsSettings() {
  return {
    ...settingsResponse({
      fallback_questions: sourced([
        { q: "Name a Gentoo package manager", answers: ["Portage", "emerge"] }
      ]),
      fallback_builtin: sourced(false)
    }),
    delivery_mode: sourced("both"),
    verify_mode: sourced("kernel"),
    timeout_seconds: sourced(240),
    verify_max_fails: sourced(3),
    verify_retry_seconds: sourced(180),
    ban_seconds: sourced(0),
    mute_seconds: sourced(3600),
    verify_invited: sourced(true)
  };
}

function deferred(): Deferred {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
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

async function expectControlsFocusable(controls: Locator): Promise<void> {
  const count = await controls.count();
  expect(count).toBeGreaterThan(0);
  for (let index = 0; index < count; index += 1) {
    const control = controls.nth(index);
    await control.focus();
    await expect(control).toBeFocused();
  }
}

async function exerciseBusyQuestionControls(
  page: Page,
  patchRequested: Promise<void>,
  patchCalls: () => number
): Promise<void> {
  await page.getByLabel("题面").first().fill("Edited while testing the busy state");
  const addQuestion = page.getByRole("button", { name: "添加选择题" });
  await addQuestion.focus();
  await page.locator("[data-questions-form]").evaluate((form) => {
    (form as HTMLFormElement).requestSubmit();
  });
  await patchRequested;

  await expect(addQuestion).toBeFocused();
  await expect(addQuestion).toHaveAttribute("aria-disabled", "true");
  const controls = page.locator(
    [
      "#questions-language-select",
      "[data-question-list-heading] > button",
      "[data-fallback-mode-options] > button",
      "[data-question-bank-editor] button",
      "[data-question-bank-editor] input",
      "[data-question-bank-editor] textarea",
      "[data-fallback-question-editor] button",
      "[data-fallback-question-editor] input",
      "[data-fallback-question-editor] textarea"
    ].join(", ")
  );
  await expectControlsFocusable(controls);
  const editorTextControls = page.locator(
    "[data-question-bank-editor] input, [data-question-bank-editor] textarea, " +
      "[data-fallback-question-editor] input, [data-fallback-question-editor] textarea"
  );
  expect(
    await editorTextControls.evaluateAll((fields) =>
      fields.every(
        (field) => (field as HTMLInputElement | HTMLTextAreaElement).readOnly
      )
    )
  ).toBe(true);
  const guardedButtons = page.locator(
    [
      "[data-question-list-heading] > button",
      "[data-fallback-mode-options] > button",
      "[data-question-bank-editor] button",
      "[data-fallback-question-editor] button"
    ].join(", ")
  );
  expect(
    await guardedButtons.evaluateAll((buttons) =>
      buttons.every((button) => button.getAttribute("aria-disabled") === "true")
    )
  ).toBe(true);

  const questionCount = await page.locator("[data-question-bank-editor] [data-question-item]").count();
  const optionCount = await page.locator("[data-question-bank-editor] [data-question-option-row]").count();
  const answerCount = await page.locator("[data-fallback-question-editor] [data-fallback-answer-row]").count();
  await addQuestion.dispatchEvent("click");
  await page.getByRole("button", { name: "添加选项" }).dispatchEvent("click");
  await page.getByRole("button", { name: "添加答案" }).dispatchEvent("click");
  await page.locator("#questions-language-select").dispatchEvent("click");
  await page.getByRole("button", { name: "内置本地化题目" }).dispatchEvent("click");
  expect(await page.locator("[data-question-bank-editor] [data-question-item]").count()).toBe(questionCount);
  expect(await page.locator("[data-question-bank-editor] [data-question-option-row]").count()).toBe(optionCount);
  expect(await page.locator("[data-fallback-question-editor] [data-fallback-answer-row]").count()).toBe(answerCount);
  await expect(page.locator("#questions-language-select")).toHaveAttribute("aria-expanded", "false");
  await expect(page.locator("[data-fallback-question-editor]")).toBeVisible();

  await page.getByRole("button", { name: "正在保存…" }).dispatchEvent("click");
  await page.waitForTimeout(100);
  expect(patchCalls()).toBe(1);
}

async function exerciseBusyVerificationControls(
  page: Page,
  patchRequested: Promise<void>,
  patchCalls: () => number
): Promise<void> {
  await page.locator("#verification-timeout-seconds").fill("241");
  const focusedSelect = page.locator("#verification-mode");
  await focusedSelect.focus();
  await page.locator("[data-verification-form]").evaluate((form) => {
    (form as HTMLFormElement).requestSubmit();
  });
  await patchRequested;

  await expect(focusedSelect).toBeFocused();
  await expect(focusedSelect).toHaveAttribute("aria-disabled", "true");
  const controls = page.locator(
    "#verification-delivery-mode, #verification-mode, [data-verification-number], #verification-invited-members"
  );
  await expectControlsFocusable(controls);
  expect(
    await page.locator("[data-verification-number]").evaluateAll((inputs) =>
      inputs.every((input) => (input as HTMLInputElement).readOnly)
    )
  ).toBe(true);

  await focusedSelect.dispatchEvent("click");
  await expect(focusedSelect).toHaveAttribute("aria-expanded", "false");
  const invited = page.locator("#verification-invited-members");
  await invited.evaluate((input) => (input as HTMLInputElement).click());
  await expect(invited).toBeChecked();

  await page.getByRole("button", { name: "正在保存…" }).dispatchEvent("click");
  await page.waitForTimeout(100);
  expect(patchCalls()).toBe(2);
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

test("busy question and verification controls keep focus and ignore interaction", async ({ page }) => {
  const questionPatchRequested = deferred();
  const questionPatchResponse = deferred();
  const verificationPatchRequested = deferred();
  const verificationPatchResponse = deferred();
  let patchCalls = 0;
  let currentSettings = busyControlsSettings();

  await openQuestions(
    page,
    async (route) => fulfillJSON(route, currentSettings),
    async (route) => {
      patchCalls += 1;
      const requested =
        patchCalls === 1
          ? questionPatchRequested
          : patchCalls === 2
            ? verificationPatchRequested
            : undefined;
      const response =
        patchCalls === 1
          ? questionPatchResponse
          : patchCalls === 2
            ? verificationPatchResponse
            : undefined;
      if (!requested || !response) {
        throw new Error(`Unexpected extra settings PATCH request ${patchCalls}`);
      }

      requested.resolve();
      await response.promise;
      currentSettings = { ...currentSettings, revision: currentSettings.revision + 1 };
      await fulfillJSON(route, currentSettings);
    }
  );

  await exerciseBusyQuestionControls(
    page,
    questionPatchRequested.promise,
    () => patchCalls
  );
  questionPatchResponse.resolve();
  await expect(page.locator('[data-questions-feedback="saved"]')).toBeVisible();

  await page.goto(`/verification?group=${selectedGroupID}`);
  await expect(page.locator("[data-verification-page]")).toHaveAttribute(
    "data-verification-state",
    "loaded"
  );
  await exerciseBusyVerificationControls(
    page,
    verificationPatchRequested.promise,
    () => patchCalls
  );
  verificationPatchResponse.resolve();
  await expect(page.locator("[data-verification-feedback]")).toContainText(
    "已保存验证设置"
  );
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
  await expect(page.locator("#questions-language-select")).toHaveAttribute("data-value", "zh");
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
