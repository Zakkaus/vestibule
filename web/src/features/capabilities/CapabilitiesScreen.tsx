import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import { consoleApi, useConsoleSession } from "../../app/session";
import { Icon } from "../../icons";
import type { IconName } from "../../icons";
import type { ApiRequestError } from "../../lib/api";
import {
  loadCapabilitySettings,
  saveCapabilitySettings,
  type CapabilitySettings
} from "./api";
import { CapabilityCard, SourceMeta } from "./CapabilityCard";

type CapabilitiesScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; settings: CapabilitySettings }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

type SaveFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; messageKey: string }>;

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "capabilities.errors.authenticationExpired",
  authentication_invalid: "capabilities.errors.authenticationInvalid",
  chat_access_denied: "capabilities.errors.accessDenied",
  chat_access_unavailable: "capabilities.errors.accessUnavailable",
  chat_not_found: "capabilities.errors.chatNotFound",
  csrf_invalid: "capabilities.errors.csrfInvalid",
  invalid_settings: "capabilities.errors.invalidSettings",
  settings_unavailable: "capabilities.errors.settingsUnavailable"
};

const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_not_found: true
};

function capabilityErrorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "capabilities.errors.network";
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
  iconName,
  role,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  iconName: IconName;
  role?: "alert";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();

  return (
    <section
      data-slot="card"
      data-capabilities-state-card={id}
      role={role}
      aria-labelledby={`capabilities-${id}-title`}
    >
      <h2 id={`capabilities-${id}-title`}>
        <span data-state-heading>
          <Icon name={iconName} />
          {t(titleKey)}
        </span>
      </h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

export function CapabilitiesScreen() {
  const { t } = useTranslation();
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const selectedGroupID = searchParams.get("group");
  const chatID =
    selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID) ? selectedGroupID : undefined;
  const groupSearch = chatID ? `?${new URLSearchParams({ group: chatID }).toString()}` : "";
  const [screenState, setScreenState] = useState<CapabilitiesScreenState>({ kind: "loading" });
  const [draftEnabled, setDraftEnabled] = useState<boolean | null>(null);
  const [restoringEnabled, setRestoringEnabled] = useState(false);
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<SaveFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const saveSequenceRef = useRef(0);

  useEffect(() => {
    const scope = `${session.state}:${chatID ?? ""}:${reloadVersion}`;
    activeScopeRef.current = scope;
    saveSequenceRef.current += 1;
    setDraftEnabled(null);
    setRestoringEnabled(false);
    setSaving(false);
    setFeedback(null);
    let active = true;

    if (session.state === "loading" || session.state === "checking-groups") {
      setScreenState({ kind: "loading" });
      return () => {
        active = false;
      };
    }
    if (session.state === "blocked" || session.state === "groups-unavailable") {
      setScreenState({ kind: "unavailable", error: session.error });
      return () => {
        active = false;
      };
    }
    if (session.state === "no-groups") {
      setScreenState({ kind: "no-groups" });
      return () => {
        active = false;
      };
    }
    if (!chatID) {
      setScreenState({ kind: "group-required" });
      return () => {
        active = false;
      };
    }

    setScreenState({ kind: "loading" });
    void loadCapabilitySettings(consoleApi, chatID).then((result) => {
      if (!active || activeScopeRef.current !== scope) {
        return;
      }
      if (!result.ok) {
        setScreenState({ kind: "unavailable", error: result.error });
        return;
      }
      setScreenState({ kind: "loaded", settings: result.data });
      setDraftEnabled(result.data.enabled.value);
    });

    return () => {
      active = false;
    };
  }, [chatID, reloadVersion, session]);

  const settings = screenState.kind === "loaded" ? screenState.settings : null;
  const hasChanges =
    settings !== null &&
    draftEnabled !== null &&
    (restoringEnabled || draftEnabled !== settings.enabled.value);
  const enabledReadOnly =
    settings === null || settings.enabled.source === "user file" || restoringEnabled || saving;

  function toggleEnabled(): void {
    if (draftEnabled === null || enabledReadOnly) {
      return;
    }
    setDraftEnabled(!draftEnabled);
    setFeedback(null);
  }

  function toggleRestore(): void {
    if (!settings || settings.enabled.source !== "chat override" || saving) {
      return;
    }
    setDraftEnabled(settings.enabled.value);
    setRestoringEnabled((current) => !current);
    setFeedback(null);
  }

  function discardChanges(): void {
    if (!settings || saving) {
      return;
    }
    setDraftEnabled(settings.enabled.value);
    setRestoringEnabled(false);
    setFeedback(null);
  }

  async function submitSettings(): Promise<void> {
    if (!settings || draftEnabled === null || !chatID || !hasChanges || saving) {
      return;
    }

    const scope = activeScopeRef.current;
    const sequence = saveSequenceRef.current + 1;
    saveSequenceRef.current = sequence;
    setSaving(true);
    setFeedback(null);
    const result = await saveCapabilitySettings(consoleApi, chatID, settings.revision, {
      enabled: restoringEnabled ? null : draftEnabled
    });

    if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
      return;
    }
    if (result.ok) {
      setScreenState({ kind: "loaded", settings: result.data });
      setDraftEnabled(result.data.enabled.value);
      setRestoringEnabled(false);
      setSaving(false);
      setFeedback({ kind: "saved" });
      return;
    }
    if (result.error.kind === "api" && result.error.code === "settings_conflict") {
      setSaving(false);
      setFeedback({ kind: "conflict" });
      return;
    }
    if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
      setSaving(false);
      setScreenState({ kind: "unavailable", error: result.error });
      return;
    }
    setSaving(false);
    setFeedback({
      kind: "error",
      messageKey: capabilityErrorMessageKey(result.error, "capabilities.errors.saveUnavailable")
    });
  }

  return (
    <section
      data-capabilities-page
      data-capabilities-state={screenState.kind}
      aria-busy={screenState.kind === "loading" || saving ? true : undefined}
      aria-labelledby="capabilities-title"
    >
      <header data-page-heading>
        <h1 id="capabilities-title">{t("capabilities.title")}</h1>
        <p>{t("capabilities.description")}</p>
      </header>

      {screenState.kind === "loading" ? (
        <StateCard
          id="loading"
          titleKey="capabilities.loading.title"
          descriptionKey="capabilities.loading.description"
          iconName="loaderCircle"
        />
      ) : null}
      {screenState.kind === "group-required" ? (
        <StateCard
          id="group-required"
          titleKey="capabilities.groupRequired.title"
          descriptionKey="capabilities.groupRequired.description"
          iconName="usersRound"
        >
          <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
            <Icon name="usersRound" />
            {t("capabilities.groupRequired.select")}
          </Link>
        </StateCard>
      ) : null}
      {screenState.kind === "no-groups" ? (
        <StateCard
          id="no-groups"
          titleKey="capabilities.noGroups.title"
          descriptionKey="capabilities.noGroups.description"
          iconName="usersRound"
        />
      ) : null}
      {screenState.kind === "unavailable" ? (
        <StateCard
          id="unavailable"
          titleKey="capabilities.unavailable.title"
          descriptionKey={capabilityErrorMessageKey(
            screenState.error,
            "capabilities.errors.loadUnavailable"
          )}
          iconName="circleAlert"
          role="alert"
        >
          <button
            type="button"
            data-slot="button"
            data-variant="outline"
            data-size="sm"
            onClick={() => setReloadVersion((version) => version + 1)}
          >
            <Icon name="refreshCw" />
            {t("capabilities.actions.retry")}
          </button>
        </StateCard>
      ) : null}

      {settings && draftEnabled !== null ? (
        <form
          data-capabilities-form
          onSubmit={(event) => {
            event.preventDefault();
            void submitSettings();
          }}
          aria-busy={saving || undefined}
        >
          <div data-capabilities-list>
            <CapabilityCard
              id="verification"
              titleKey="capabilities.verification.title"
              summaryKey="capabilities.verification.summary"
              enabled={draftEnabled}
              source={settings.enabled.source}
              onKey="capabilities.verification.enabled"
              offKey="capabilities.verification.disabled"
              detailsPath="/verification"
              detailsKey="capabilities.verification.details"
              groupSearch={groupSearch}
              sourceMeta={
                <SourceMeta
                  source={settings.enabled.source}
                  pending={hasChanges}
                  restoring={restoringEnabled}
                  saving={saving}
                  allowRestore
                  onToggleRestore={toggleRestore}
                />
              }
              control={
                <button
                  type="button"
                  role="switch"
                  data-slot="switch"
                  aria-checked={draftEnabled}
                  aria-disabled={enabledReadOnly ? "true" : undefined}
                  aria-labelledby="capabilities-verification-title"
                  aria-describedby="capabilities-verification-summary"
                  onClick={toggleEnabled}
                />
              }
            />
            <CapabilityCard
              id="antispam"
              titleKey="capabilities.antispam.title"
              summaryKey="capabilities.antispam.summary"
              enabled={settings.antispam_enabled.value}
              source={settings.antispam_enabled.source}
              onKey="capabilities.antispam.enabled"
              offKey="capabilities.antispam.disabled"
              detailsPath="/moderation"
              detailsKey="capabilities.antispam.details"
              groupSearch={groupSearch}
            />
          </div>

          {hasChanges ? (
            <aside data-slot="card" data-capabilities-savebar aria-label={t("capabilities.save.label")}>
              <span aria-live="polite">{t("capabilities.save.unsaved")}</span>
              <span data-capabilities-save-actions>
                <button
                  type="button"
                  data-slot="button"
                  data-variant="outline"
                  data-size="sm"
                  aria-disabled={saving ? "true" : undefined}
                  onClick={discardChanges}
                >
                  <Icon name="trash2" />
                  {t("capabilities.actions.discard")}
                </button>
                <button
                  type="submit"
                  data-slot="button"
                  data-variant="primary"
                  data-size="sm"
                  aria-disabled={saving ? "true" : undefined}
                  onClick={(event) => {
                    if (saving) {
                      event.preventDefault();
                    }
                  }}
                >
                  <Icon name="save" />
                  {t(saving ? "capabilities.actions.saving" : "capabilities.actions.save")}
                </button>
              </span>
            </aside>
          ) : null}
        </form>
      ) : null}

      {feedback ? (
        <div
          data-slot="card"
          data-capabilities-feedback={feedback.kind}
          data-status={feedback.kind === "saved" ? "ok" : "error"}
          role={feedback.kind === "saved" ? "status" : "alert"}
          aria-atomic="true"
        >
          <span>
            <Icon name={feedback.kind === "saved" ? "circleCheck" : "circleAlert"} />
            {t(
              feedback.kind === "saved"
                ? "capabilities.feedback.saved"
                : feedback.kind === "conflict"
                  ? "capabilities.feedback.conflict"
                  : feedback.messageKey
            )}
          </span>
          {feedback.kind === "conflict" ? (
            <button
              type="button"
              data-slot="button"
              data-variant="outline"
              data-size="sm"
              onClick={() => setReloadVersion((version) => version + 1)}
            >
              <Icon name="refreshCw" />
              {t("capabilities.actions.reload")}
            </button>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
