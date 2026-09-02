import { type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { AppSelect, type AppSelectOption } from "../../components/AppSelect";
import { StatusBadge } from "../../components/StatusBadge";
import { Icon } from "../../icons";
import { FallbackQuestionEditor } from "./FallbackQuestionEditor";
import { QuestionBankEditor } from "./QuestionBankEditor";
import {
  questionLanguages,
  type QuestionLanguage,
  type QuestionSettingField,
  type QuestionSettings,
  type SettingSource
} from "./api";
import {
  newQuestionDraft,
  newShortQuestionDraft,
  type QuestionsDraft,
  type QuestionsValidation
} from "./model";

export type QuestionsSettingsFormProps = Readonly<{
  settings: QuestionSettings;
  draft: QuestionsDraft;
  validation: QuestionsValidation;
  restored: ReadonlySet<QuestionSettingField>;
  saving: boolean;
  hasChanges: boolean;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onDraftChange: (draft: QuestionsDraft, changedFields: readonly QuestionSettingField[]) => void;
  onRestore: (field: QuestionSettingField) => void;
  onRestoreFallback: () => void;
}>;

type SourceMetaProps = Readonly<{
  source: SettingSource;
  restoring: boolean;
  labelKey?: string;
  onRestore: () => void;
}>;

type SectionProps = Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  children: ReactNode;
}>;

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "questions.source.factoryDefault",
  "user file": "questions.source.userFile",
  "chat override": "questions.source.chatOverride"
};

const languageMessageKeys: Readonly<Record<QuestionLanguage, string>> = {
  zh: "questions.language.zh",
  "zh-Hant": "questions.language.zhHant",
  en: "questions.language.en"
};

function QuestionSection({ id, titleKey, descriptionKey, children }: SectionProps) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-questions-section aria-labelledby={`${id}-title`}>
      <div data-questions-section-heading>
        <h2 id={`${id}-title`}>{t(titleKey)}</h2>
        <p>{t(descriptionKey)}</p>
      </div>
      {children}
    </section>
  );
}

function SourceMeta({ source, restoring, labelKey = "questions.source.value", onRestore }: SourceMetaProps) {
  const { t } = useTranslation();
  return (
    <div data-question-setting-meta>
      <StatusBadge tone="neutral">
        {t(labelKey, { source: t(sourceMessageKeys[source]) })}
      </StatusBadge>
      {restoring ? (
        <StatusBadge tone="pending">{t("questions.source.restoring")}</StatusBadge>
      ) : null}
      {source === "chat override" && !restoring ? (
        <button
          type="button"
          data-slot="button"
          data-variant="link"
          data-size="sm"
          onClick={onRestore}
        >
          <Icon name="rotateCcw" />
          {t("questions.actions.restore")}
        </button>
      ) : null}
    </div>
  );
}

function LanguageSection({
  settings,
  draft,
  restored,
  saving,
  onDraftChange,
  onRestore
}: Omit<QuestionsSettingsFormProps, "validation" | "hasChanges" | "onSubmit" | "onRestoreFallback">) {
  const { t } = useTranslation();
  const descriptionID = "questions-language-description";
  const languageOptions: readonly AppSelectOption<QuestionLanguage>[] = questionLanguages.map((language) => ({
    label: t(languageMessageKeys[language]),
    value: language
  }));
  return (
    <QuestionSection
      id="questions-language"
      titleKey="questions.language.title"
      descriptionKey="questions.language.description"
    >
      <div data-slot="setting" data-question-setting="lang">
        <div data-question-setting-copy>
          <label htmlFor="questions-language-select">{t("questions.language.label")}</label>
          <p id={descriptionID}>{t("questions.language.settingDescription")}</p>
          <SourceMeta
            source={settings.lang.source}
            restoring={restored.has("lang")}
            onRestore={() => onRestore("lang")}
          />
        </div>
        <div data-question-setting-control>
          <AppSelect
            aria-label={t("questions.language.label")}
            id="questions-language-select"
            value={draft.lang}
            aria-disabled={saving ? "true" : undefined}
            aria-describedby={descriptionID}
            options={languageOptions}
            onValueChange={(value) =>
              onDraftChange(
                { ...draft, lang: value },
                ["lang"]
              )
            }
          />
        </div>
      </div>
    </QuestionSection>
  );
}

function QuestionBankSection({
  settings,
  draft,
  validation,
  restored,
  saving,
  onDraftChange,
  onRestore
}: Omit<QuestionsSettingsFormProps, "hasChanges" | "onSubmit" | "onRestoreFallback">) {
  const { t } = useTranslation();
  return (
    <QuestionSection
      id="questions-bank"
      titleKey="questions.questionBank.title"
      descriptionKey="questions.questionBank.description"
    >
      <SourceMeta
        source={settings.questions.source}
        restoring={restored.has("questions")}
        onRestore={() => onRestore("questions")}
      />
      <div data-questions-concurrency role="note">
        <strong>{t("questions.concurrency.title")}</strong>
        <p>{t("questions.concurrency.description")}</p>
      </div>
      <div data-question-list-heading>
        <p>{t("questions.questionBank.count", { count: draft.questions.length })}</p>
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          aria-disabled={saving ? "true" : undefined}
          onClick={() => {
            if (!saving) {
              onDraftChange(
                { ...draft, questions: [...draft.questions, newQuestionDraft()] },
                ["questions"]
              );
            }
          }}
        >
          <Icon name="plus" />
          {t("questions.actions.addQuestion")}
        </button>
      </div>
      <QuestionBankEditor
        questions={draft.questions}
        errors={validation.questionErrors}
        readOnly={saving}
        onChange={(questions) => onDraftChange({ ...draft, questions }, ["questions"])}
      />
    </QuestionSection>
  );
}

function FallbackSources({
  settings,
  restored,
  onRestore
}: Readonly<{
  settings: QuestionSettings;
  restored: ReadonlySet<QuestionSettingField>;
  onRestore: () => void;
}>) {
  const { t } = useTranslation();
  const restoring = restored.has("fallback_builtin") || restored.has("fallback_questions");
  const canRestore =
    settings.fallback_builtin.source === "chat override" ||
    settings.fallback_questions.source === "chat override";
  return (
    <div data-fallback-sources>
      <StatusBadge tone="neutral">
        {t("questions.source.fallbackMode", {
          source: t(sourceMessageKeys[settings.fallback_builtin.source])
        })}
      </StatusBadge>
      <StatusBadge tone="neutral">
        {t("questions.source.fallbackBank", {
          source: t(sourceMessageKeys[settings.fallback_questions.source])
        })}
      </StatusBadge>
      {restoring ? (
        <StatusBadge tone="pending">{t("questions.source.restoring")}</StatusBadge>
      ) : null}
      {canRestore && !restoring ? (
        <button
          type="button"
          data-slot="button"
          data-variant="link"
          data-size="sm"
          onClick={onRestore}
        >
          <Icon name="rotateCcw" />
          {t("questions.actions.restoreFallback")}
        </button>
      ) : null}
    </div>
  );
}

function FallbackMode({
  draft,
  saving,
  onDraftChange
}: Pick<QuestionsSettingsFormProps, "draft" | "saving" | "onDraftChange">) {
  const { t } = useTranslation();
  const descriptionID = "questions-fallback-mode-description";
  function chooseMode(fallbackBuiltin: boolean): void {
    const fallbackQuestions =
      !fallbackBuiltin && draft.fallbackQuestions.length === 0
        ? [newShortQuestionDraft()]
        : draft.fallbackQuestions;
    onDraftChange({ ...draft, fallbackBuiltin, fallbackQuestions }, ["fallback_builtin"]);
  }

  return (
    <fieldset data-fallback-mode aria-describedby={descriptionID}>
      <legend>{t("questions.fallback.modeLabel")}</legend>
      <p id={descriptionID}>{t("questions.fallback.modeDescription")}</p>
      <div data-fallback-mode-options>
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          aria-pressed={draft.fallbackBuiltin}
          aria-describedby={descriptionID}
          aria-disabled={saving ? "true" : undefined}
          onClick={() => {
            if (!saving) {
              chooseMode(true);
            }
          }}
        >
          <Icon name="bookOpen" />
          {t("questions.fallback.builtin")}
        </button>
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          aria-pressed={!draft.fallbackBuiltin}
          aria-describedby={descriptionID}
          aria-disabled={saving ? "true" : undefined}
          onClick={() => {
            if (!saving) {
              chooseMode(false);
            }
          }}
        >
          <Icon name="pencil" />
          {t("questions.fallback.custom")}
        </button>
      </div>
    </fieldset>
  );
}

function FallbackSection(props: Omit<QuestionsSettingsFormProps, "hasChanges" | "onSubmit">) {
  const { t } = useTranslation();
  const { settings, draft, validation, restored, saving, onDraftChange, onRestoreFallback } = props;
  return (
    <QuestionSection
      id="questions-fallback"
      titleKey="questions.fallback.title"
      descriptionKey="questions.fallback.description"
    >
      <FallbackSources settings={settings} restored={restored} onRestore={onRestoreFallback} />
      <FallbackMode draft={draft} saving={saving} onDraftChange={onDraftChange} />
      {draft.fallbackBuiltin ? (
        <p data-fallback-builtin-note>{t("questions.fallback.builtinDescription")}</p>
      ) : (
        <>
          <div data-question-list-heading>
            <p>{t("questions.fallback.count", { count: draft.fallbackQuestions.length })}</p>
            <button
              type="button"
              data-slot="button"
              data-variant="outline"
              data-size="sm"
              aria-disabled={saving ? "true" : undefined}
              onClick={() => {
                if (!saving) {
                  onDraftChange(
                    {
                      ...draft,
                      fallbackQuestions: [...draft.fallbackQuestions, newShortQuestionDraft()]
                    },
                    ["fallback_questions"]
                  );
                }
              }}
            >
              <Icon name="plus" />
              {t("questions.actions.addFallback")}
            </button>
          </div>
          <FallbackQuestionEditor
            questions={draft.fallbackQuestions}
            errors={validation.fallbackQuestionErrors}
            listErrorKey={validation.fallbackListError}
            readOnly={saving}
            onChange={(fallbackQuestions) =>
              onDraftChange({ ...draft, fallbackQuestions }, ["fallback_questions"])
            }
          />
        </>
      )}
    </QuestionSection>
  );
}

export function QuestionsSettingsForm(props: QuestionsSettingsFormProps) {
  const { t } = useTranslation();
  return (
    <form data-questions-form onSubmit={props.onSubmit}>
      <LanguageSection {...props} />
      <QuestionBankSection {...props} />
      <FallbackSection {...props} />
      <footer data-slot="card" data-questions-savebar>
        <p>{t(props.hasChanges ? "questions.save.dirty" : "questions.save.clean")}</p>
        <button
          type="submit"
          data-slot="button"
          data-variant="primary"
          aria-disabled={props.saving ? "true" : undefined}
          disabled={!props.hasChanges}
        >
          <Icon name="save" />
          {t(props.saving ? "questions.actions.saving" : "questions.actions.save")}
        </button>
      </footer>
    </form>
  );
}
