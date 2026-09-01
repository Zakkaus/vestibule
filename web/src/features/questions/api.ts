import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export const settingSources = ["factory default", "user file", "chat override"] as const;
export type SettingSource = (typeof settingSources)[number];

export const questionLanguages = ["zh", "zh-Hant", "en"] as const;
export type QuestionLanguage = (typeof questionLanguages)[number];

export const questionSettingFields = [
  "questions",
  "fallback_questions",
  "fallback_builtin",
  "lang"
] as const;
export type QuestionSettingField = (typeof questionSettingFields)[number];

export type Question = Readonly<{
  q: string;
  options: readonly string[];
  answer: number;
}>;

export type ShortQuestion = Readonly<{
  q: string;
  answers: readonly string[];
}>;

export type SourcedSetting<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

export type QuestionSettings = Readonly<{
  revision: number;
  questions: SourcedSetting<readonly Question[]>;
  fallback_questions: SourcedSetting<readonly ShortQuestion[]>;
  fallback_builtin: SourcedSetting<boolean>;
  lang: SourcedSetting<QuestionLanguage>;
}>;

export type QuestionSettingsChanges = Partial<{
  questions: readonly Question[] | null;
  fallback_questions: readonly ShortQuestion[] | null;
  fallback_builtin: boolean | null;
  lang: QuestionLanguage | null;
}>;

function choiceFromPayload<T extends readonly string[]>(
  value: unknown,
  choices: T
): T[number] | undefined {
  return typeof value === "string" && (choices as readonly string[]).includes(value)
    ? (value as T[number])
    : undefined;
}

function integerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : undefined;
}

function stringArrayFromPayload(value: unknown): readonly string[] | undefined {
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string")) {
    return undefined;
  }
  return value as string[];
}

function questionFromPayload(value: unknown): Question | undefined {
  const question = objectFromPayload(value);
  if (!question) {
    return undefined;
  }

  const options = stringArrayFromPayload(question.options);
  const answer = integerFromPayload(question.answer);
  if (typeof question.q !== "string" || !options || answer === undefined) {
    return undefined;
  }

  return { q: question.q, options, answer };
}

function shortQuestionFromPayload(value: unknown): ShortQuestion | undefined {
  const question = objectFromPayload(value);
  if (!question) {
    return undefined;
  }

  const answers = stringArrayFromPayload(question.answers);
  if (typeof question.q !== "string" || !answers) {
    return undefined;
  }

  return { q: question.q, answers };
}

function arrayFromPayload<T>(
  value: unknown,
  parseItem: (item: unknown) => T | undefined
): readonly T[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const items: T[] = [];
  for (const item of value) {
    const parsed = parseItem(item);
    if (parsed === undefined) {
      return undefined;
    }
    items.push(parsed);
  }
  return items;
}

function sourcedSettingFromPayload<T>(
  value: unknown,
  parseValue: (candidate: unknown) => T | undefined
): SourcedSetting<T> | undefined {
  const setting = objectFromPayload(value);
  if (!setting) {
    return undefined;
  }

  const parsedValue = parseValue(setting.value);
  const source = choiceFromPayload(setting.source, settingSources);
  if (parsedValue === undefined || source === undefined) {
    return undefined;
  }

  return { value: parsedValue, source };
}

export function questionSettingsFromPayload(payload: unknown): QuestionSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const revision = integerFromPayload(response.revision);
  const questions = sourcedSettingFromPayload(response.questions, (value) =>
    arrayFromPayload(value, questionFromPayload)
  );
  const fallbackQuestions = sourcedSettingFromPayload(response.fallback_questions, (value) =>
    arrayFromPayload(value, shortQuestionFromPayload)
  );
  const fallbackBuiltin = sourcedSettingFromPayload(response.fallback_builtin, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const lang = sourcedSettingFromPayload(response.lang, (value) =>
    choiceFromPayload(value, questionLanguages)
  );

  if (
    revision === undefined ||
    revision < 0 ||
    !questions ||
    !fallbackQuestions ||
    !fallbackBuiltin ||
    !lang
  ) {
    return undefined;
  }

  return {
    revision,
    questions,
    fallback_questions: fallbackQuestions,
    fallback_builtin: fallbackBuiltin,
    lang
  };
}

export function loadQuestionSettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<QuestionSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    parse: questionSettingsFromPayload
  });
}

export function saveQuestionSettings(
  transport: ApiTransport,
  chatID: string,
  expectedRevision: number,
  changes: QuestionSettingsChanges
): Promise<ApiResult<QuestionSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    method: "PATCH",
    body: {
      expected_revision: expectedRevision,
      changes
    },
    parse: questionSettingsFromPayload
  });
}
