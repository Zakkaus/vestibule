import { Button } from "@react-spectrum/s2/Button";
import { Text } from "@react-spectrum/s2";
import { ToggleButton } from "@react-spectrum/s2/ToggleButton";
import { useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import { consoleApi } from "../../app/session";
import { AppSelect, type AppSelectOption } from "../../components/AppSelect";
import type { ApiRequestError, ApiResult } from "../../lib/api";
import { Icon } from "../../icons";
import {
  testQuestion,
  type QuestionSettings,
  type QuestionTrialCollection,
  type QuestionTrialRequest,
  type QuestionTrialResult
} from "./api";

type QuestionTrialProps = Readonly<{
  chatID: string;
  settings: QuestionSettings;
  onReload: () => void;
}>;

type TrialSelection = Readonly<{
  collection: QuestionTrialCollection;
  questionIndex: number;
  choice: number;
  answer: string;
  chooseCollection: (value: QuestionTrialCollection) => void;
  chooseQuestion: (value: string) => void;
  chooseChoice: (index: number) => void;
  changeAnswer: (answer: string) => void;
}>;

type TrialController = TrialSelection & Readonly<{
  pending: boolean;
  result: QuestionTrialResult | null;
  error: ApiRequestError | null;
  question:
    | QuestionSettings["questions"]["value"][number]
    | QuestionSettings["fallback_questions"]["value"][number]
    | undefined;
  questions:
    | QuestionSettings["questions"]["value"]
    | QuestionSettings["fallback_questions"]["value"];
  submit: (event: FormEvent<HTMLFormElement>) => void;
}>;

const trialErrorKeys: Readonly<Record<string, string>> = {
  authentication_expired: "questions.trial.errors.authenticationExpired",
  authentication_invalid: "questions.trial.errors.authenticationInvalid",
  chat_access_denied: "questions.trial.errors.accessDenied",
  chat_not_found: "questions.trial.errors.chatNotFound",
  csrf_invalid: "questions.trial.errors.csrfInvalid",
  invalid_rule: "questions.trial.errors.invalidRule",
  chat_access_unavailable: "questions.trial.errors.accessUnavailable",
  rule_not_found: "questions.trial.errors.questionNotFound",
  settings_conflict: "questions.trial.errors.conflict",
  settings_unavailable: "questions.trial.errors.settingsUnavailable"
};

function trialErrorMessageKey(error: ApiRequestError): string {
  if (error.kind === "network") {
    return "questions.trial.errors.network";
  }
  if (error.kind === "api") {
    return trialErrorKeys[error.code] ?? "questions.trial.errors.unavailable";
  }
  return "questions.trial.errors.invalidResponse";
}

function submitQuestionTrial(
  chatID: string,
  request: QuestionTrialRequest,
  sequence: number,
  sequenceRef: { current: number },
  onResponse: (response: ApiResult<QuestionTrialResult>) => void
): void {
  void testQuestion(consoleApi, chatID, request).then((response) => {
    if (sequenceRef.current === sequence) {
      onResponse(response);
    }
  });
}

function useTrialSelection(onChange: () => void): TrialSelection {
  const [collection, setCollection] = useState<QuestionTrialCollection>("questions");
  const [questionIndex, setQuestionIndex] = useState(0);
  const [choice, setChoice] = useState(0);
  const [answer, setAnswer] = useState("");
  function chooseCollection(value: QuestionTrialCollection): void {
    if (value === collection) {
      return;
    }
    setCollection(value);
    setQuestionIndex(0);
    setChoice(0);
    setAnswer("");
    onChange();
  }
  function chooseQuestion(value: string): void {
    setQuestionIndex(Number.parseInt(value, 10));
    setChoice(0);
    setAnswer("");
    onChange();
  }
  function chooseChoice(index: number): void {
    setChoice(index);
    onChange();
  }
  function changeAnswer(value: string): void {
    setAnswer(value);
    onChange();
  }
  return {
    collection,
    questionIndex,
    choice,
    answer,
    chooseCollection,
    chooseQuestion,
    chooseChoice,
    changeAnswer
  };
}

function useQuestionTrial(chatID: string, settings: QuestionSettings): TrialController {
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<QuestionTrialResult | null>(null);
  const [error, setError] = useState<ApiRequestError | null>(null);
  const sequenceRef = useRef(0);
  function clearResult(): void {
    sequenceRef.current += 1;
    setPending(false);
    setResult(null);
    setError(null);
  }
  const selection = useTrialSelection(clearResult);
  const questions = selection.collection === "questions"
    ? settings.questions.value
    : settings.fallback_questions.value;
  const question = questions[selection.questionIndex];
  function submit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    if (!question || pending) {
      return;
    }
    const sequence = sequenceRef.current + 1;
    sequenceRef.current = sequence;
    setPending(true);
    setResult(null);
    setError(null);
    const request: QuestionTrialRequest = selection.collection === "questions"
      ? {
          collection: selection.collection,
          questionIndex: selection.questionIndex,
          expectedRevision: settings.revision,
          choice: selection.choice
        }
      : {
          collection: selection.collection,
          questionIndex: selection.questionIndex,
          expectedRevision: settings.revision,
          answer: selection.answer
        };
    submitQuestionTrial(chatID, request, sequence, sequenceRef, (response) => {
      setPending(false);
      if (response.ok) {
        setResult(response.data);
      } else {
        setError(response.error);
      }
    });
  }
  return { ...selection, questions, question, pending, result, error, submit };
}

function TrialResultNotice({ result }: Readonly<{ result: QuestionTrialResult }>) {
  const { t } = useTranslation();
  return (
    <p
      role="status"
      aria-live="polite"
    >
      <Icon name={result.correct ? "circleCheck" : "circleAlert"} />
      {t(result.correct ? "questions.trial.correct" : "questions.trial.incorrect")}
    </p>
  );
}

function TrialErrorNotice({ error, onReload }: Readonly<{ error: ApiRequestError; onReload: () => void }>) {
  const { t } = useTranslation();
  return (
    <div role="alert" aria-atomic="true">
      <p>
        <Icon name="circleAlert" /> {t(trialErrorMessageKey(error))}
      </p>
      <Button variant="secondary" onPress={onReload}>
        <Icon name="refreshCw" />
        <Text>{t("questions.trial.retry")}</Text>
      </Button>
    </div>
  );
}

function TrialAnswer({ controller }: Readonly<{ controller: TrialController }>) {
  const { t } = useTranslation();
  const { collection, question, choice, answer, chooseChoice, changeAnswer } = controller;
  if (!question) {
    return <p>{t("questions.trial.noQuestions")}</p>;
  }
  return (
    <div>
      <p>{question.q}</p>
      {collection === "questions" && "options" in question ? (
        <fieldset>
          <legend>{t("questions.trial.choiceLabel")}</legend>
          <div data-question-option-list>
            {question.options.map((option, index) => (
              <ToggleButton
                key={`${index}-${option}`}
                isSelected={choice === index}
                onChange={(selected) => { if (selected) chooseChoice(index); }}
              >
                <Text>{option}</Text>
              </ToggleButton>
            ))}
          </div>
        </fieldset>
      ) : (
        <div data-question-field>
          <label htmlFor="questions-trial-answer">{t("questions.trial.answerLabel")}</label>
          <input
            id="questions-trial-answer"
            data-slot="input"
            type="text"
            value={answer}
            onChange={(event) => changeAnswer(event.currentTarget.value)}
          />
        </div>
      )}
    </div>
  );
}

function TrialControls({ controller, onReload }: Readonly<{ controller: TrialController; onReload: () => void }>) {
  const { t } = useTranslation();
  const { collection, questionIndex, pending, questions, chooseCollection, chooseQuestion, submit, result, error } = controller;
  const collectionOptions: readonly AppSelectOption<QuestionTrialCollection>[] = [
    { label: t("questions.trial.multipleChoice"), value: "questions" },
    { label: t("questions.trial.fallback"), value: "fallback_questions" }
  ];
  const questionOptions: readonly AppSelectOption<string>[] = questions.map((item, index) => ({
    label: `${index + 1}. ${item.q}`,
    value: String(index)
  }));
  return (
    <form data-questions-form onSubmit={submit}>
      <div data-slot="setting">
        <div data-question-setting-copy>
          <label htmlFor="questions-trial-collection">{t("questions.trial.collectionLabel")}</label>
        </div>
        <div data-question-setting-control>
          <AppSelect
            aria-label={t("questions.trial.collectionLabel")}
            id="questions-trial-collection"
            value={collection}
            options={collectionOptions}
            onValueChange={chooseCollection}
          />
        </div>
      </div>
      <div data-slot="setting">
        <div data-question-setting-copy>
          <label htmlFor="questions-trial-question">{t("questions.trial.questionLabel")}</label>
        </div>
        <div data-question-setting-control>
          <AppSelect
            aria-label={t("questions.trial.questionLabel")}
            id="questions-trial-question"
            value={String(questionIndex)}
            options={questionOptions}
            onValueChange={chooseQuestion}
          />
        </div>
      </div>
      <TrialAnswer controller={controller} />
      <div data-question-list-heading>
        {/* Secondary: the screen's one accent action is saving the bank. */}
        <Button
          type="submit"
          variant="secondary"
          aria-disabled={pending ? "true" : undefined}
          isPending={pending}
          isDisabled={!controller.question}
        >
          <Icon name="arrowRight" />
          <Text>{t(pending ? "questions.trial.submitting" : "questions.trial.submit")}</Text>
        </Button>
        {result ? <TrialResultNotice result={result} /> : null}
        {error ? <TrialErrorNotice error={error} onReload={onReload} /> : null}
      </div>
    </form>
  );
}

export function QuestionTrial({ chatID, settings, onReload }: QuestionTrialProps) {
  const { t } = useTranslation();
  const controller = useQuestionTrial(chatID, settings);
  return (
    <section data-slot="card" data-questions-section aria-labelledby="questions-trial-title">
      <div data-questions-section-heading>
        <h2 id="questions-trial-title">{t("questions.trial.title")}</h2>
        <p>{t("questions.trial.description")}</p>
      </div>
      <p role="note">{t("questions.trial.savedOnly")}</p>
      <TrialControls controller={controller} onReload={onReload} />
    </section>
  );
}
