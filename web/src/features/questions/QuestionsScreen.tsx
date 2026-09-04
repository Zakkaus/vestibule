import { type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import type { ApiRequestError } from "../../lib/api";
import { Icon, type IconName } from "../../icons";
import { QuestionsSettingsForm } from "./QuestionsSettingsForm";
import {
  useQuestionSettings,
  type QuestionsController,
  type QuestionsFeedback
} from "./useQuestionSettings";

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "questions.errors.authenticationExpired",
  authentication_invalid: "questions.errors.authenticationInvalid",
  chat_access_denied: "questions.errors.accessDenied",
  chat_access_unavailable: "questions.errors.accessUnavailable",
  chat_not_found: "questions.errors.chatNotFound",
  csrf_invalid: "questions.errors.csrfInvalid",
  invalid_settings: "questions.errors.invalidSettings",
  settings_unavailable: "questions.errors.settingsUnavailable"
};

function questionErrorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "questions.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? fallback;
  }
  return fallback;
}

function StateCard({
  id,
  titleKey,
  descriptionKey,
  icon,
  role,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  icon: IconName;
  role?: "alert";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-questions-state-card={id}
      role={role}
      aria-labelledby={`questions-${id}-title`}
    >
      <h2 id={`questions-${id}-title`}>
        <span data-state-heading>
          <Icon name={icon} />
          {t(titleKey)}
        </span>
      </h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

function QuestionsStateContent({ controller }: Readonly<{ controller: QuestionsController }>) {
  const { t } = useTranslation();
  const { state } = controller;
  if (state.kind === "loading") {
    return (
      <StateCard
        id="loading"
        titleKey="questions.loading.title"
        icon="loaderCircle"
        descriptionKey="questions.loading.description"
      />
    );
  }
  if (state.kind === "group-required") {
    return (
      <StateCard
        id="group-required"
        titleKey="questions.groupRequired.title"
        icon="usersRound"
        descriptionKey="questions.groupRequired.description"
      >
        <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
          <Icon name="usersRound" />
          {t("questions.groupRequired.select")}
        </Link>
      </StateCard>
    );
  }
  if (state.kind === "no-groups") {
    return (
      <StateCard
        id="no-groups"
        titleKey="questions.noGroups.title"
        icon="usersRound"
        descriptionKey="questions.noGroups.description"
      />
    );
  }
  if (state.kind === "unavailable") {
    return (
      <StateCard
        id="unavailable"
        titleKey="questions.unavailable.title"
        icon="circleAlert"
        descriptionKey={questionErrorMessageKey(state.error, "questions.errors.loadUnavailable")}
        role="alert"
      >
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          onClick={controller.reload}
        >
          <Icon name="refreshCw" />
          {t("questions.unavailable.retry")}
        </button>
      </StateCard>
    );
  }
  if (!controller.draft) {
    return null;
  }

  function submit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    void controller.save();
  }

  return (
    <QuestionsSettingsForm
      settings={state.settings}
      draft={controller.draft}
      validation={controller.validation}
      restored={controller.restored}
      saving={controller.saving}
      hasChanges={controller.hasChanges}
      onSubmit={submit}
      onDraftChange={controller.updateDraft}
      onRestore={controller.restore}
      onRestoreFallback={controller.restoreFallback}
    />
  );
}

function QuestionsFeedbackNotice({
  feedback,
  onReload
}: Readonly<{ feedback: QuestionsFeedback; onReload: () => void }>) {
  const { t } = useTranslation();
  const messageKey =
    feedback.kind === "saved"
      ? "questions.feedback.saved"
      : feedback.kind === "conflict"
        ? "questions.feedback.conflict"
        : questionErrorMessageKey(feedback.error, "questions.errors.saveUnavailable");
  const isError = feedback.kind !== "saved";
  const reloadable = feedback.kind === "error" && feedback.error.kind === "network";
  return (
    <div
      data-questions-feedback={feedback.kind}
      data-tone={isError ? "error" : "ok"}
      role={isError ? "alert" : "status"}
      aria-atomic="true"
    >
      <span data-state-heading>
        <Icon name={isError ? "circleAlert" : "circleCheck"} />
        {t(messageKey)}
      </span>
      {reloadable ? (
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          onClick={onReload}
        >
          <Icon name="refreshCw" />
          {t("questions.actions.reload")}
        </button>
      ) : null}
    </div>
  );
}

export function QuestionsScreen() {
  const { t } = useTranslation();
  const controller = useQuestionSettings();
  return (
    <section
      data-questions-page
      data-questions-state={controller.state.kind}
      aria-busy={controller.state.kind === "loading" || controller.saving ? true : undefined}
      aria-labelledby="questions-title"
    >
      <header data-page-heading>
        <h1 id="questions-title">{t("questions.title")}</h1>
        <p>{t("questions.description")}</p>
      </header>
      <QuestionsStateContent controller={controller} />
      {controller.feedback ? (
        <QuestionsFeedbackNotice feedback={controller.feedback} onReload={controller.reload} />
      ) : null}
    </section>
  );
}
