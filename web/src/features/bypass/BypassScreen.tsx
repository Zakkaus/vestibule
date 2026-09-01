import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import type { ApiRequestError } from "../../lib/api";
import { BypassSettingsForm } from "./BypassSettingsForm";
import {
  useBypassSettings,
  type BypassController
} from "./useBypassSettings";

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "bypass.errors.authenticationExpired",
  authentication_invalid: "bypass.errors.authenticationInvalid",
  chat_access_denied: "bypass.errors.accessDenied",
  chat_access_unavailable: "bypass.errors.accessUnavailable",
  chat_not_found: "bypass.errors.chatNotFound",
  csrf_invalid: "bypass.errors.csrfInvalid",
  invalid_settings: "bypass.errors.invalidSettings",
  settings_unavailable: "bypass.errors.settingsUnavailable"
};

function bypassErrorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "bypass.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? fallback;
  }
  return fallback;
}

type StateCardProps = Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  role?: "alert";
  live?: "polite";
  children?: ReactNode;
}>;

function BypassStateCard({
  id,
  titleKey,
  descriptionKey,
  role,
  live,
  children
}: StateCardProps) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-bypass-state-card={id}
      role={role}
      aria-live={live}
      aria-labelledby={`bypass-${id}-title`}
    >
      <h2 id={`bypass-${id}-title`}>{t(titleKey)}</h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

function BypassStateContent({ controller }: Readonly<{ controller: BypassController }>) {
  const { t } = useTranslation();
  const { state, evaluation } = controller;
  if (state.kind === "ready") {
    if (!evaluation) {
      return null;
    }
    return (
      <BypassSettingsForm
        state={state}
        evaluation={evaluation}
        onEditText={controller.editText}
        onEditFailOpen={controller.editFailOpen}
        onSetRestoring={controller.setRestoring}
        onDiscard={controller.discard}
        onSave={controller.save}
        errorMessageKey={(error) => bypassErrorMessageKey(error, "bypass.errors.saveUnavailable")}
      />
    );
  }
  if (state.kind === "loading") {
    return (
      <BypassStateCard
        id="loading"
        titleKey="bypass.loading.title"
        descriptionKey="bypass.loading.description"
        live="polite"
      />
    );
  }
  if (state.kind === "group-required") {
    return (
      <BypassStateCard
        id="group-required"
        titleKey="bypass.groupRequired.title"
        descriptionKey="bypass.groupRequired.description"
      >
        <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
          {t("bypass.groupRequired.select")}
        </Link>
      </BypassStateCard>
    );
  }
  if (state.kind === "no-groups") {
    return (
      <BypassStateCard
        id="no-groups"
        titleKey="bypass.noGroups.title"
        descriptionKey="bypass.noGroups.description"
      />
    );
  }
  return (
    <BypassStateCard
      id="unavailable"
      titleKey="bypass.unavailable.title"
      descriptionKey={bypassErrorMessageKey(state.error, "bypass.errors.loadUnavailable")}
      role="alert"
    >
      <button
        type="button"
        data-slot="button"
        data-variant="outline"
        data-size="sm"
        onClick={controller.reload}
      >
        {t("bypass.unavailable.retry")}
      </button>
    </BypassStateCard>
  );
}

export function BypassScreen() {
  const { t } = useTranslation();
  const controller = useBypassSettings();
  const { state } = controller;
  const screenState = state.kind === "ready" ? "loaded" : state.kind;
  return (
    <section
      data-bypass-page
      data-bypass-state={screenState}
      aria-busy={state.kind === "loading" || (state.kind === "ready" && state.saving) || undefined}
      aria-labelledby="bypass-title"
    >
      <header data-page-heading>
        <h1 id="bypass-title">{t("bypass.title")}</h1>
        <p>{t("bypass.description")}</p>
      </header>
      <BypassStateContent controller={controller} />
    </section>
  );
}
