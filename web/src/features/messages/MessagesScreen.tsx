import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import { useConsoleSession } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import { Icon, type IconName } from "../../icons";
import { MessageSettingsForm } from "./MessageSettingsForm";
import { RulesPanel } from "./RulesPanel";
import {
  useMessageSettings,
  type MessageSettingsFeedback
} from "./useMessageSettings";
import { useMessageRules } from "./useMessageRules";

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "messages.errors.authenticationExpired",
  authentication_invalid: "messages.errors.authenticationInvalid",
  chat_access_denied: "messages.errors.accessDenied",
  chat_access_unavailable: "messages.errors.accessUnavailable",
  chat_not_found: "messages.errors.chatNotFound",
  csrf_invalid: "messages.errors.csrfInvalid",
  invalid_settings: "messages.errors.invalidSettings",
  settings_unavailable: "messages.errors.settingsUnavailable",
  invalid_rule: "messages.errors.invalidRule",
  rule_not_found: "messages.errors.ruleNotFound",
  rule_conflict: "messages.errors.ruleConflict",
  rules_unavailable: "messages.errors.rulesUnavailable"
};

function errorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "messages.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? fallback;
  }
  return fallback;
}

function StateCard({
  id,
  icon,
  titleKey,
  descriptionKey,
  role,
  children
}: Readonly<{
  id: string;
  icon: IconName;
  titleKey: string;
  descriptionKey: string;
  role?: "alert";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();
  const titleID = `messages-${id}-title`;

  return (
    <section data-slot="card" data-messages-state-card={id} role={role} aria-labelledby={titleID}>
      <h2 id={titleID} data-state-heading>
        <Icon name={icon} />
        {t(titleKey)}
      </h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

function SettingsFeedbackNotice({ feedback }: Readonly<{ feedback: MessageSettingsFeedback }>) {
  const { t } = useTranslation();
  const messageKey =
    feedback.kind === "saved"
      ? "messages.settingsFeedback.saved"
      : feedback.kind === "conflict"
        ? "messages.settingsFeedback.conflict"
        : errorMessageKey(feedback.error, "messages.errors.saveSettingsUnavailable");

  return (
    <div
      data-messages-settings-feedback
      data-tone={feedback.kind === "saved" ? "ok" : "error"}
      role={feedback.kind === "saved" ? "status" : "alert"}
    >
      <Icon name={feedback.kind === "saved" ? "circleCheck" : "circleAlert"} />
      {t(messageKey)}
    </div>
  );
}

export function MessagesScreen() {
  const { t } = useTranslation();
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const selectedGroupID = searchParams.get("group");
  const chatID =
    selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID) ? selectedGroupID : undefined;
  const ready = session.state === "ready" && chatID !== undefined;
  const settings = useMessageSettings(chatID, ready);
  const rules = useMessageRules(chatID, ready);
  const pageBusy =
    ready &&
    (settings.state.kind === "loading" || rules.state.kind === "loading" || settings.saving || rules.busy !== null);

  let content: ReactNode;
  if (session.state === "loading" || session.state === "checking-groups") {
    content = (
      <StateCard
        id="loading"
        icon="loaderCircle"
        titleKey="messages.loading.title"
        descriptionKey="messages.loading.description"
      />
    );
  } else if (session.state === "blocked" || session.state === "groups-unavailable") {
    content = (
      <StateCard
        id="session-unavailable"
        icon="circleAlert"
        titleKey="messages.unavailable.title"
        descriptionKey={errorMessageKey(session.error, "messages.errors.loadUnavailable")}
        role="alert"
      />
    );
  } else if (session.state === "no-groups") {
    content = (
      <StateCard
        id="no-groups"
        icon="usersRound"
        titleKey="messages.noGroups.title"
        descriptionKey="messages.noGroups.description"
      />
    );
  } else if (!chatID) {
    content = (
      <StateCard
        id="group-required"
        icon="usersRound"
        titleKey="messages.groupRequired.title"
        descriptionKey="messages.groupRequired.description"
      >
        <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
          <Icon name="usersRound" />
          {t("messages.groupRequired.select")}
        </Link>
      </StateCard>
    );
  } else {
    const rulesFeedback = rules.feedback
      ? {
          tone:
            rules.feedback.kind === "saved-item" || rules.feedback.kind === "saved-order"
              ? ("ok" as const)
              : ("error" as const),
          content:
            rules.feedback.kind === "saved-item"
              ? t("messages.rules.feedback.itemSaved")
              : rules.feedback.kind === "saved-order"
                ? t("messages.rules.feedback.orderSaved", { collection: rules.feedback.collection })
                : rules.feedback.kind === "conflict"
                  ? t("messages.rules.feedback.conflict")
                  : t(errorMessageKey(rules.feedback.error, "messages.errors.saveRulesUnavailable"))
        }
      : null;

    content = (
      <>
        {rules.state.kind === "loading" ? (
          <StateCard
            id="rules-loading"
            icon="loaderCircle"
            titleKey="messages.rules.loading.title"
            descriptionKey="messages.rules.loading.description"
          />
        ) : null}
        {rules.state.kind === "unavailable" ? (
          <StateCard
            id="rules-unavailable"
            icon="circleAlert"
            titleKey="messages.rules.unavailable.title"
            descriptionKey={errorMessageKey(rules.state.error, "messages.errors.loadRulesUnavailable")}
            role="alert"
          >
            <button
              type="button"
              data-slot="button"
              data-variant="outline"
              data-size="sm"
              onClick={rules.retry}
            >
              <Icon name="refreshCw" />
              {t("messages.actions.retry")}
            </button>
          </StateCard>
        ) : null}
        {rules.state.kind === "loaded" ? (
          <RulesPanel
            items={rules.state.items}
            busy={rules.busy}
            feedback={rulesFeedback}
            onToggle={rules.toggle}
            onMove={rules.move}
          />
        ) : null}

        {settings.state.kind === "loading" ? (
          <StateCard
            id="settings-loading"
            icon="loaderCircle"
            titleKey="messages.settings.loading.title"
            descriptionKey="messages.settings.loading.description"
          />
        ) : null}
        {settings.state.kind === "unavailable" ? (
          <StateCard
            id="settings-unavailable"
            icon="circleAlert"
            titleKey="messages.settings.unavailable.title"
            descriptionKey={errorMessageKey(
              settings.state.error,
              "messages.errors.loadSettingsUnavailable"
            )}
            role="alert"
          >
            <button
              type="button"
              data-slot="button"
              data-variant="outline"
              data-size="sm"
              onClick={settings.retry}
            >
              <Icon name="refreshCw" />
              {t("messages.actions.retry")}
            </button>
          </StateCard>
        ) : null}
        {settings.state.kind === "loaded" && settings.draft && settings.evaluation ? (
          <>
            <MessageSettingsForm
              settings={settings.state.settings}
              draft={settings.draft}
              evaluation={settings.evaluation}
              restoring={settings.restoring}
              saving={settings.saving}
              onSubmit={settings.submit}
              onDraftChange={settings.setDraft}
              onSetRestoring={settings.setRestoring}
            />
            {settings.feedback ? <SettingsFeedbackNotice feedback={settings.feedback} /> : null}
          </>
        ) : null}

        <section data-slot="card" data-messages-copy-gap aria-labelledby="messages-copy-gap-title">
          <h2 id="messages-copy-gap-title" data-state-heading>
            <Icon name="circleAlert" />
            {t("messages.copyGap.title")}
          </h2>
          <p>{t("messages.copyGap.description")}</p>
          <ul>
            <li>{t("messages.copyGap.joinLeave")}</li>
            <li>{t("messages.copyGap.verification")}</li>
          </ul>
        </section>
      </>
    );
  }

  return (
    <section
      data-messages-page
      data-messages-state={ready ? "loaded" : session.state}
      aria-busy={pageBusy || undefined}
      aria-labelledby="messages-title"
    >
      <header data-page-heading>
        <h1 id="messages-title">
          <Icon name="messagesSquare" />
          {t("messages.title")}
        </h1>
        <p>{t("messages.description")}</p>
      </header>
      {content}
    </section>
  );
}
