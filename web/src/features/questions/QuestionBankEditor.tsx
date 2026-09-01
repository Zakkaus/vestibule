import { useTranslation } from "react-i18next";

import type { QuestionDraft, QuestionItemErrors } from "./model";

export type QuestionBankEditorProps = Readonly<{
  questions: readonly QuestionDraft[];
  errors: Readonly<Record<string, QuestionItemErrors>>;
  disabled: boolean;
  onChange: (questions: readonly QuestionDraft[]) => void;
}>;

type QuestionItemProps = Readonly<{
  question: QuestionDraft;
  number: number;
  errors?: QuestionItemErrors;
  disabled: boolean;
  onChange: (question: QuestionDraft) => void;
  onDelete: () => void;
}>;

type QuestionPromptProps = Readonly<{
  question: QuestionDraft;
  errorKey?: string;
  disabled: boolean;
  onChange: (question: QuestionDraft) => void;
}>;

type QuestionOptionsProps = Readonly<{
  question: QuestionDraft;
  errors?: QuestionItemErrors;
  disabled: boolean;
  onChange: (question: QuestionDraft) => void;
}>;

type QuestionOptionRowProps = Readonly<{
  question: QuestionDraft;
  option: string;
  optionIndex: number;
  disabled: boolean;
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

function QuestionPrompt({ question, errorKey, disabled, onChange }: QuestionPromptProps) {
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
        disabled={disabled}
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
  disabled,
  onChange
}: QuestionOptionRowProps) {
  const { t } = useTranslation();
  const optionID = `question-${question.id}-option-${optionIndex}`;

  return (
    <div data-question-option-row>
      <input
        type="radio"
        name={`question-${question.id}-answer`}
        checked={question.answer === optionIndex}
        disabled={disabled}
        aria-label={t("questions.questionBank.correctAnswerFor", { number: optionIndex + 1 })}
        onChange={() => onChange({ ...question, answer: optionIndex })}
      />
      <div data-question-option-field>
        <label htmlFor={optionID}>
          {t("questions.questionBank.option", { number: optionIndex + 1 })}
        </label>
        <input
          id={optionID}
          data-slot="input"
          value={option}
          disabled={disabled}
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
        disabled={disabled}
        aria-label={t("questions.actions.removeOptionFor", { number: optionIndex + 1 })}
        onClick={() => onChange(removeOption(question, optionIndex))}
      >
        {t("questions.actions.removeOption")}
      </button>
    </div>
  );
}

function QuestionOptionsEditor({ question, errors, disabled, onChange }: QuestionOptionsProps) {
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
            disabled={disabled}
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
        disabled={disabled}
        onClick={() => onChange({ ...question, options: [...question.options, ""] })}
      >
        {t("questions.actions.addOption")}
      </button>
    </fieldset>
  );
}

function QuestionItem({
  question,
  number,
  errors,
  disabled,
  onChange,
  onDelete
}: QuestionItemProps) {
  const { t } = useTranslation();

  return (
    <fieldset data-question-item>
      <legend>{t("questions.questionBank.itemTitle", { number })}</legend>
      <div data-question-item-actions>
        <button
          type="button"
          data-slot="button"
          data-variant="destructive"
          data-size="sm"
          disabled={disabled}
          onClick={onDelete}
        >
          {t("questions.actions.deleteQuestion")}
        </button>
      </div>
      <QuestionPrompt
        question={question}
        errorKey={errors?.q}
        disabled={disabled}
        onChange={onChange}
      />
      <QuestionOptionsEditor
        question={question}
        errors={errors}
        disabled={disabled}
        onChange={onChange}
      />
    </fieldset>
  );
}

export function QuestionBankEditor({
  questions,
  errors,
  disabled,
  onChange
}: QuestionBankEditorProps) {
  const { t } = useTranslation();

  return (
    <div data-question-bank-editor>
      {questions.length === 0 ? (
        <p data-question-empty>{t("questions.questionBank.empty")}</p>
      ) : (
        <div data-question-list>
          {questions.map((question, index) => (
            <QuestionItem
              key={question.id}
              question={question}
              number={index + 1}
              errors={errors[question.id]}
              disabled={disabled}
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
