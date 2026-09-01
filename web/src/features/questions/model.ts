import type {
  Question,
  QuestionLanguage,
  QuestionSettingField,
  QuestionSettings,
  QuestionSettingsChanges,
  ShortQuestion
} from "./api";

export type QuestionDraft = Readonly<{
  id: string;
  q: string;
  options: readonly string[];
  answer: number;
}>;

export type ShortQuestionDraft = Readonly<{
  id: string;
  q: string;
  answers: readonly string[];
}>;

export type QuestionsDraft = Readonly<{
  questions: readonly QuestionDraft[];
  fallbackQuestions: readonly ShortQuestionDraft[];
  fallbackBuiltin: boolean;
  lang: QuestionLanguage;
}>;

export type QuestionItemErrors = Readonly<{
  q?: string;
  options?: string;
  answer?: string;
}>;

export type ShortQuestionItemErrors = Readonly<{
  q?: string;
  answers?: string;
}>;

export type QuestionsValidation = Readonly<{
  questionErrors: Readonly<Record<string, QuestionItemErrors>>;
  fallbackQuestionErrors: Readonly<Record<string, ShortQuestionItemErrors>>;
  fallbackListError?: string;
  values?: Readonly<{
    questions: readonly Question[];
    fallbackQuestions: readonly ShortQuestion[];
  }>;
}>;

let nextDraftSequence = 0;

function draftID(kind: "question" | "fallback"): string {
  nextDraftSequence += 1;
  return `${kind}-${nextDraftSequence}`;
}

export function newQuestionDraft(): QuestionDraft {
  return {
    id: draftID("question"),
    q: "",
    options: ["", ""],
    answer: 0
  };
}

export function newShortQuestionDraft(): ShortQuestionDraft {
  return {
    id: draftID("fallback"),
    q: "",
    answers: [""]
  };
}

export function draftFromSettings(settings: QuestionSettings): QuestionsDraft {
  return {
    questions: settings.questions.value.map((question) => ({
      ...question,
      id: draftID("question"),
      options: [...question.options]
    })),
    fallbackQuestions: settings.fallback_questions.value.map((question) => ({
      ...question,
      id: draftID("fallback"),
      answers: [...question.answers]
    })),
    fallbackBuiltin: settings.fallback_builtin.value,
    lang: settings.lang.value
  };
}

function questionValues(draft: QuestionsDraft): readonly Question[] {
  return draft.questions.map(({ q, options, answer }) => ({ q, options: [...options], answer }));
}

function fallbackQuestionValues(draft: QuestionsDraft): readonly ShortQuestion[] {
  return draft.fallbackQuestions.map(({ q, answers }) => ({ q, answers: [...answers] }));
}

export function validateQuestionsDraft(draft: QuestionsDraft): QuestionsValidation {
  const questionErrors: Record<string, QuestionItemErrors> = {};
  const fallbackQuestionErrors: Record<string, ShortQuestionItemErrors> = {};

  for (const question of draft.questions) {
    const errors: { q?: string; options?: string; answer?: string } = {};
    if (question.q.trim().length === 0) {
      errors.q = "questions.validation.questionPrompt";
    }
    if (question.options.length < 2) {
      errors.options = "questions.validation.questionOptions";
    }
    if (
      !Number.isSafeInteger(question.answer) ||
      question.answer < 0 ||
      question.answer >= question.options.length
    ) {
      errors.answer = "questions.validation.questionAnswer";
    }
    if (Object.keys(errors).length > 0) {
      questionErrors[question.id] = errors;
    }
  }

  let fallbackListError: string | undefined;
  if (!draft.fallbackBuiltin) {
    if (draft.fallbackQuestions.length === 0) {
      fallbackListError = "questions.validation.fallbackRequired";
    }
    for (const question of draft.fallbackQuestions) {
      const errors: { q?: string; answers?: string } = {};
      if (question.q.trim().length === 0) {
        errors.q = "questions.validation.fallbackPrompt";
      }
      if (question.answers.length === 0) {
        errors.answers = "questions.validation.fallbackAnswers";
      } else if (question.answers.some((answer) => answer.trim().length === 0)) {
        errors.answers = "questions.validation.fallbackAnswerBlank";
      }
      if (Object.keys(errors).length > 0) {
        fallbackQuestionErrors[question.id] = errors;
      }
    }
  }

  if (
    Object.keys(questionErrors).length > 0 ||
    Object.keys(fallbackQuestionErrors).length > 0 ||
    fallbackListError
  ) {
    return { questionErrors, fallbackQuestionErrors, fallbackListError };
  }

  return {
    questionErrors,
    fallbackQuestionErrors,
    values: {
      questions: questionValues(draft),
      fallbackQuestions: fallbackQuestionValues(draft)
    }
  };
}

function stringArraysEqual(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

function questionsEqual(left: readonly Question[], right: readonly Question[]): boolean {
  return (
    left.length === right.length &&
    left.every((question, index) => {
      const candidate = right[index];
      return (
        candidate !== undefined &&
        question.q === candidate.q &&
        question.answer === candidate.answer &&
        stringArraysEqual(question.options, candidate.options)
      );
    })
  );
}

function fallbackQuestionsEqual(
  left: readonly ShortQuestion[],
  right: readonly ShortQuestion[]
): boolean {
  return (
    left.length === right.length &&
    left.every((question, index) => {
      const candidate = right[index];
      return (
        candidate !== undefined &&
        question.q === candidate.q &&
        stringArraysEqual(question.answers, candidate.answers)
      );
    })
  );
}

export function hasQuestionDraftChanges(
  settings: QuestionSettings,
  draft: QuestionsDraft,
  restored: ReadonlySet<QuestionSettingField>
): boolean {
  if (restored.size > 0) {
    return true;
  }

  return (
    !questionsEqual(settings.questions.value, questionValues(draft)) ||
    !fallbackQuestionsEqual(settings.fallback_questions.value, fallbackQuestionValues(draft)) ||
    settings.fallback_builtin.value !== draft.fallbackBuiltin ||
    settings.lang.value !== draft.lang
  );
}

export function sparseQuestionChanges(
  settings: QuestionSettings,
  draft: QuestionsDraft,
  restored: ReadonlySet<QuestionSettingField>,
  values: NonNullable<QuestionsValidation["values"]>
): QuestionSettingsChanges {
  const changes: QuestionSettingsChanges = {};

  if (restored.has("questions")) {
    changes.questions = null;
  } else if (!questionsEqual(settings.questions.value, values.questions)) {
    changes.questions = values.questions;
  }

  if (restored.has("fallback_builtin")) {
    changes.fallback_builtin = null;
  } else if (settings.fallback_builtin.value !== draft.fallbackBuiltin) {
    changes.fallback_builtin = draft.fallbackBuiltin;
  }

  if (restored.has("fallback_questions")) {
    changes.fallback_questions = null;
  } else if (!draft.fallbackBuiltin) {
    if (!fallbackQuestionsEqual(settings.fallback_questions.value, values.fallbackQuestions)) {
      changes.fallback_questions = values.fallbackQuestions;
    }
  } else if (
    settings.fallback_builtin.value !== draft.fallbackBuiltin &&
    settings.fallback_questions.source === "chat override"
  ) {
    changes.fallback_questions = null;
  }

  if (restored.has("lang")) {
    changes.lang = null;
  } else if (settings.lang.value !== draft.lang) {
    changes.lang = draft.lang;
  }

  return changes;
}
