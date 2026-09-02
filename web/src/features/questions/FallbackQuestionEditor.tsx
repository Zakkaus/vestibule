import { useTranslation } from "react-i18next";
import { Icon } from "../../icons";

import type { ShortQuestionDraft, ShortQuestionItemErrors } from "./model";

export type FallbackQuestionEditorProps = Readonly<{
  questions: readonly ShortQuestionDraft[];
  errors: Readonly<Record<string, ShortQuestionItemErrors>>;
  listErrorKey?: string;
  disabled: boolean;
  onChange: (questions: readonly ShortQuestionDraft[]) => void;
}>;

type FallbackItemProps = Readonly<{
  question: ShortQuestionDraft;
  number: number;
  errors?: ShortQuestionItemErrors;
  disabled: boolean;
  onChange: (question: ShortQuestionDraft) => void;
  onDelete: () => void;
}>;

type AnswerRowProps = Readonly<{
  question: ShortQuestionDraft;
  answer: string;
  answerIndex: number;
  disabled: boolean;
  onChange: (question: ShortQuestionDraft) => void;
}>;

function FallbackAnswerRow({
  question,
  answer,
  answerIndex,
  disabled,
  onChange
}: AnswerRowProps) {
  const { t } = useTranslation();
  const answerID = `fallback-${question.id}-answer-${answerIndex}`;

  return (
    <div data-fallback-answer-row>
      <div data-question-option-field>
        <label htmlFor={answerID}>
          {t("questions.fallback.answer", { number: answerIndex + 1 })}
        </label>
        <input
          id={answerID}
          data-slot="input"
          value={answer}
          disabled={disabled}
          onChange={(event) => {
            const answers = [...question.answers];
            answers[answerIndex] = event.currentTarget.value;
            onChange({ ...question, answers });
          }}
        />
      </div>
      <button
        type="button"
        data-slot="button"
        data-variant="link"
        data-size="sm"
        disabled={disabled}
        aria-label={t("questions.actions.removeAnswerFor", { number: answerIndex + 1 })}
        onClick={() =>
          onChange({
            ...question,
            answers: question.answers.filter((_, index) => index !== answerIndex)
          })
        }
      >
        <Icon name="trash2" />
        {t("questions.actions.removeAnswer")}
      </button>
    </div>
  );
}

function FallbackQuestionItem({
  question,
  number,
  errors,
  disabled,
  onChange,
  onDelete
}: FallbackItemProps) {
  const { t } = useTranslation();
  const promptID = `fallback-prompt-${question.id}`;
  const promptErrorID = `${promptID}-error`;
  const answersDescriptionID = `fallback-answers-${question.id}-description`;
  const answersErrorID = `fallback-answers-${question.id}-error`;

  return (
    <section
      data-slot="card"
      data-question-item
      data-fallback-item
      aria-labelledby={`fallback-question-${question.id}-title`}
    >
      <header data-question-item-heading>
        <h3 id={`fallback-question-${question.id}-title`}>
          {t("questions.fallback.itemTitle", { number })}
        </h3>
        <button
          type="button"
          data-slot="button"
          data-variant="destructive"
          data-size="sm"
          disabled={disabled}
          onClick={onDelete}
        >
          <Icon name="trash2" />
          {t("questions.actions.deleteFallback")}
        </button>
      </header>
      <div data-question-field>
        <label htmlFor={promptID}>{t("questions.fallback.prompt")}</label>
        <textarea
          id={promptID}
          data-slot="textarea"
          value={question.q}
          disabled={disabled}
          aria-invalid={errors?.q ? "true" : undefined}
          aria-describedby={errors?.q ? promptErrorID : undefined}
          onChange={(event) => onChange({ ...question, q: event.currentTarget.value })}
        />
        {errors?.q ? (
          <p id={promptErrorID} data-slot="field-error" role="alert">
            {t(errors.q)}
          </p>
        ) : null}
      </div>
      <fieldset
        data-fallback-answers
        aria-describedby={`${answersDescriptionID}${errors?.answers ? ` ${answersErrorID}` : ""}`}
      >
        <legend>{t("questions.fallback.answers")}</legend>
        <p id={answersDescriptionID}>{t("questions.fallback.answersDescription")}</p>
        <div data-fallback-answer-list>
          {question.answers.map((answer, answerIndex) => (
            <FallbackAnswerRow
              key={`fallback-${question.id}-answer-${answerIndex}`}
              question={question}
              answer={answer}
              answerIndex={answerIndex}
              disabled={disabled}
              onChange={onChange}
            />
          ))}
        </div>
        {errors?.answers ? (
          <p id={answersErrorID} data-slot="field-error" role="alert">
            {t(errors.answers)}
          </p>
        ) : null}
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          disabled={disabled}
          onClick={() => onChange({ ...question, answers: [...question.answers, ""] })}
        >
          <Icon name="plus" />
          {t("questions.actions.addAnswer")}
        </button>
      </fieldset>
    </section>
  );
}

export function FallbackQuestionEditor({
  questions,
  errors,
  listErrorKey,
  disabled,
  onChange
}: FallbackQuestionEditorProps) {
  const { t } = useTranslation();

  return (
    <div data-fallback-question-editor>
      {listErrorKey ? (
        <p data-slot="field-error" role="alert">
          {t(listErrorKey)}
        </p>
      ) : null}
      <div data-question-list>
        {questions.map((question, index) => (
          <FallbackQuestionItem
            key={question.id}
            question={question}
            number={index + 1}
            errors={errors[question.id]}
            disabled={disabled}
            onChange={(next) =>
              onChange(questions.map((item) => (item.id === question.id ? next : item)))
            }
            onDelete={() => {
              if (window.confirm(t("questions.fallback.confirmDelete", { number: index + 1 }))) {
                onChange(questions.filter((item) => item.id !== question.id));
              }
            }}
          />
        ))}
      </div>
    </div>
  );
}
