import { useTranslation } from "react-i18next";
import { Icon } from "../../icons";

import type { QuestionDraft, QuestionItemErrors } from "./model";

export type QuestionBankEditorProps = Readonly<{
  questions: readonly QuestionDraft[];
  errors: Readonly<Record<string, QuestionItemErrors>>;
  readOnly: boolean;
  onChange: (questions: readonly QuestionDraft[]) => void;
}>;

type QuestionItemProps = Readonly<{
  question: QuestionDraft;
  number: number;
  errors?: QuestionItemErrors;
  readOnly: boolean;
  onChange: (question: QuestionDraft) => void;
  onDelete: () => void;
}>;

type QuestionPromptProps = Readonly<{
  question: QuestionDraft;
  errorKey?: string;
  readOnly: boolean;
  onChange: (question: QuestionDraft) => void;
}>;

type QuestionOptionsProps = Readonly<{
  question: QuestionDraft;
  errors?: QuestionItemErrors;
  readOnly: boolean;
  onChange: (question: QuestionDraft) => void;
}>;

type QuestionOptionRowProps = Readonly<{
  question: QuestionDraft;
  option: string;
  optionIndex: number;
  readOnly: boolean;
  onChange: (question: QuestionDraft) => void;
}>;

function removeOption(question: QuestionDraft, optionIndex: number): QuestionDraft {
  const options = question.options.filter((_, index) => index !== optionIndex);
  let answer = question.answer;
  if (optionIndex < answer) {
    answer -= 1;
  } else if (optionIndex === answer) {
    answer = 0;
  }
  if (answer >= options.length) {
    answer = Math.max(0, options.length - 1);
  }
  return { ...question, options, answer };
}

function QuestionPrompt({ question, errorKey, readOnly, onChange }: QuestionPromptProps) {
  const { t } = useTranslation();
  const promptID = `question-prompt-${question.id}`;
  const errorID = `${promptID}-error`;

  return (
    <div data-question-field>
      <label htmlFor={promptID}>{t("questions.questionBank.prompt")}</label>
      <textarea
        id={promptID}
        data-slot="textarea"
        value={question.q}
        readOnly={readOnly}
        aria-invalid={errorKey ? "true" : undefined}
        aria-describedby={errorKey ? errorID : undefined}
        onChange={(event) => onChange({ ...question, q: event.currentTarget.value })}
      />
      {errorKey ? (
        <p id={errorID} data-slot="field-error" role="alert">
          {t(errorKey)}
        </p>
      ) : null}
    </div>
  );
}

function QuestionOptionRow({
  question,
  option,
  optionIndex,
  readOnly,
  onChange
}: QuestionOptionRowProps) {
  const { t } = useTranslation();
  const optionID = `question-${question.id}-option-${optionIndex}`;

  return (
    <div data-question-option-row>
      <button
        type="button"
        data-slot="button"
        data-variant="outline"
        data-size="sm"
        aria-pressed={question.answer === optionIndex}
        aria-label={t("questions.questionBank.correctAnswerFor", { number: optionIndex + 1 })}
        aria-disabled={readOnly ? "true" : undefined}
        onClick={() => {
          if (!readOnly) {
            onChange({ ...question, answer: optionIndex });
          }
        }}
      >
        <Icon name="circleCheck" />
        {t("questions.actions.selectCorrectAnswer")}
      </button>
      <div data-question-option-field>
        <label htmlFor={optionID}>
          {t("questions.questionBank.option", { number: optionIndex + 1 })}
        </label>
        <input
          id={optionID}
          data-slot="input"
          value={option}
          readOnly={readOnly}
          onChange={(event) => {
            const options = [...question.options];
            options[optionIndex] = event.currentTarget.value;
            onChange({ ...question, options });
          }}
        />
      </div>
      <button
        type="button"
        data-slot="button"
        data-variant="link"
        data-size="sm"
        aria-disabled={readOnly ? "true" : undefined}
        aria-label={t("questions.actions.removeOptionFor", { number: optionIndex + 1 })}
        onClick={() => {
          if (!readOnly) {
            onChange(removeOption(question, optionIndex));
          }
        }}
      >
        <Icon name="trash2" />
        {t("questions.actions.removeOption")}
      </button>
    </div>

  );
}

function QuestionOptionsEditor({ question, errors, readOnly, onChange }: QuestionOptionsProps) {
  const { t } = useTranslation();
  const descriptionID = `question-options-${question.id}-description`;
  const optionsErrorID = `question-options-${question.id}-error`;
  const answerErrorID = `question-answer-${question.id}-error`;
  const describedBy = [
    descriptionID,
    errors?.options ? optionsErrorID : undefined,
    errors?.answer ? answerErrorID : undefined
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <fieldset data-question-options aria-describedby={describedBy}>
      <legend>{t("questions.questionBank.options")}</legend>
      <p id={descriptionID}>{t("questions.questionBank.optionsDescription")}</p>
      <div data-question-option-list>
        {question.options.map((option, optionIndex) => (
          <QuestionOptionRow
            key={`question-${question.id}-option-${optionIndex}`}
            question={question}
            option={option}
            optionIndex={optionIndex}
            readOnly={readOnly}
            onChange={onChange}
          />
        ))}
      </div>
      {errors?.options ? (
        <p id={optionsErrorID} data-slot="field-error" role="alert">
          {t(errors.options)}
        </p>
      ) : null}
      {errors?.answer ? (
        <p id={answerErrorID} data-slot="field-error" role="alert">
          {t(errors.answer)}
        </p>
      ) : null}
      <button
        type="button"
        data-slot="button"
        data-variant="outline"
        data-size="sm"
        aria-disabled={readOnly ? "true" : undefined}
        onClick={() => {
          if (!readOnly) {
            onChange({ ...question, options: [...question.options, ""] });
          }
        }}
      >
        <Icon name="plus" />
        {t("questions.actions.addOption")}
      </button>
    </fieldset>
  );
}

function QuestionItem({
  question,
  number,
  errors,
  readOnly,
  onChange,
  onDelete
}: QuestionItemProps) {
  const { t } = useTranslation();

  return (
    <section
      data-slot="card"
      data-question-item
      aria-labelledby={`question-${question.id}-title`}
    >
      <header data-question-item-heading>
        <h3 id={`question-${question.id}-title`}>
          {t("questions.questionBank.itemTitle", { number })}
        </h3>
        <button
          type="button"
          data-slot="button"
          data-variant="destructive"
          data-size="sm"
          aria-disabled={readOnly ? "true" : undefined}
          onClick={() => {
            if (!readOnly) {
              onDelete();
            }
          }}
        >
          <Icon name="trash2" />
          {t("questions.actions.deleteQuestion")}
        </button>
      </header>
      <QuestionPrompt
        question={question}
        errorKey={errors?.q}
        readOnly={readOnly}
        onChange={onChange}
      />
      <QuestionOptionsEditor
        question={question}
        errors={errors}
        readOnly={readOnly}
        onChange={onChange}
      />
    </section>
  );
}

export function QuestionBankEditor({
  questions,
  errors,
  readOnly,
  onChange
}: QuestionBankEditorProps) {
  const { t } = useTranslation();

  return (
    <div data-question-bank-editor>
      {questions.length === 0 ? (
        <p data-question-empty>
          <span data-state-heading>
            <Icon name="inbox" />
            {t("questions.questionBank.empty")}
          </span>
        </p>
      ) : (
        <div data-question-list>
          {questions.map((question, index) => (
            <QuestionItem
              key={question.id}
              question={question}
              number={index + 1}
              errors={errors[question.id]}
              readOnly={readOnly}
              onChange={(next) =>
                onChange(questions.map((item) => (item.id === question.id ? next : item)))
              }
              onDelete={() => {
                if (window.confirm(t("questions.questionBank.confirmDelete", { number: index + 1 }))) {
                  onChange(questions.filter((item) => item.id !== question.id));
                }
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}
