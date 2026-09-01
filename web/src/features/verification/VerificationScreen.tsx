import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import { consoleApi, useConsoleSession } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import {
  loadVerificationSettings,
  saveVerificationSettings,
  type VerificationSettingField,
  type VerificationSettings
} from "./api";
import {
  hasDraftChanges,
  settingsDraft,
  sparseChanges,
  validateDraft,
  type DraftSettings,
  type FieldErrors
} from "./form";
import { VerificationSettingsForm } from "./VerificationSettingsForm";

type VerificationScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; settings: VerificationSettings }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

type SaveFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; messageKey: string }>;

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "verification.errors.authenticationExpired",
  authentication_invalid: "verification.errors.authenticationInvalid",
  chat_access_denied: "verification.errors.accessDenied",
  chat_access_unavailable: "verification.errors.accessUnavailable",
  chat_not_found: "verification.errors.chatNotFound",
  csrf_invalid: "verification.errors.csrfInvalid",
  invalid_settings: "verification.errors.invalidSettings",
  settings_unavailable: "verification.errors.settingsUnavailable"
};

const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_not_found: true
};

function verificationErrorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "verification.errors.network";
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
  role,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  role?: "alert";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();

  return (
    <section
      data-slot="card"
      data-verification-state-card={id}
      role={role}
      aria-labelledby={`verification-${id}-title`}
    >
      <h2 id={`verification-${id}-title`}>{t(titleKey)}</h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

export function VerificationScreen() {
  const { t } = useTranslation();
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const selectedGroupID = searchParams.get("group");
  const chatID =
    selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID) ? selectedGroupID : undefined;
  const [screenState, setScreenState] = useState<VerificationScreenState>({ kind: "loading" });
  const [draft, setDraft] = useState<DraftSettings | null>(null);
  const [restored, setRestored] = useState<ReadonlySet<VerificationSettingField>>(new Set());
  const [reloadVersion, setReloadVersion] = useState(0);
  const [attemptedSave, setAttemptedSave] = useState(false);
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<SaveFeedback | null>(null);
  const activeScopeRef = useRef("");
  const saveSequenceRef = useRef(0);

  useEffect(() => {
    const scope = `${session.state}:${chatID ?? ""}:${reloadVersion}`;
    activeScopeRef.current = scope;
    saveSequenceRef.current += 1;
    setSaving(false);
    setDraft(null);
    setRestored(new Set());
    setAttemptedSave(false);
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
    void loadVerificationSettings(consoleApi, chatID).then((result) => {
      if (!active || activeScopeRef.current !== scope) {
        return;
      }
      if (!result.ok) {
        setScreenState({ kind: "unavailable", error: result.error });
        return;
      }
      setScreenState({ kind: "loaded", settings: result.data });
      setDraft(settingsDraft(result.data));
    });

    return () => {
      active = false;
    };
  }, [chatID, reloadVersion, session]);

  const validation = draft ? validateDraft(draft) : { errors: {} };
  const hasChanges =
    screenState.kind === "loaded" && draft
      ? hasDraftChanges(screenState.settings, draft, restored)
      : false;
  const changes =
    screenState.kind === "loaded" && validation.values
      ? sparseChanges(screenState.settings, validation.values, restored)
      : {};
  const errors: FieldErrors = attemptedSave ? validation.errors : {};

  function updateDraft<K extends keyof DraftSettings>(field: K, value: DraftSettings[K]): void {
    setDraft((current) => (current ? { ...current, [field]: value } : current));
    setRestored((current) => {
      const next = new Set(current);
      next.delete(field);
      return next;
    });
    setFeedback(null);
  }

  function restoreSetting(field: VerificationSettingField): void {
    if (screenState.kind !== "loaded" || screenState.settings[field].source !== "chat override") {
      return;
    }

    const inheritedDraft = settingsDraft(screenState.settings);
    setDraft((current) =>
      current ? ({ ...current, [field]: inheritedDraft[field] } as DraftSettings) : current
    );
    setRestored((current) => new Set(current).add(field));
    setFeedback(null);
  }

  async function submitSettings(): Promise<void> {
    if (screenState.kind !== "loaded" || !draft || !chatID || saving) {
      return;
    }

    setAttemptedSave(true);
    if (!validation.values || !hasChanges) {
      return;
    }

    const scope = activeScopeRef.current;
    const sequence = saveSequenceRef.current + 1;
    saveSequenceRef.current = sequence;
    setSaving(true);
    setFeedback(null);
    const result = await saveVerificationSettings(
      consoleApi,
      chatID,
      screenState.settings.revision,
      changes
    );

    if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
      return;
    }
    if (result.ok) {
      setScreenState({ kind: "loaded", settings: result.data });
      setDraft(settingsDraft(result.data));
      setRestored(new Set());
      setAttemptedSave(false);
      setFeedback({ kind: "saved" });
      setSaving(false);
      return;
    }
    if (result.error.kind === "api" && result.error.code === "settings_conflict") {
      const currentSettings = await loadVerificationSettings(consoleApi, chatID);
      if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
        return;
      }
      if (currentSettings.ok) {
        setScreenState({ kind: "loaded", settings: currentSettings.data });
        setDraft(settingsDraft(currentSettings.data));
        setRestored(new Set());
        setAttemptedSave(false);
        setFeedback({ kind: "conflict" });
        setSaving(false);
        return;
      }
      setScreenState({ kind: "unavailable", error: currentSettings.error });
      setSaving(false);
      return;
    }
    if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
      setScreenState({ kind: "unavailable", error: result.error });
      setSaving(false);
      return;
    }
    setFeedback({
      kind: "error",
      messageKey: verificationErrorMessageKey(result.error, "verification.errors.saveUnavailable")
    });
    setSaving(false);
  }

  function submit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    void submitSettings();
  }

  return (
    <section
      data-verification-page
      data-verification-state={screenState.kind}
      aria-busy={screenState.kind === "loading" || saving ? true : undefined}
      aria-labelledby="verification-title"
    >
      <header data-page-heading>
        <h1 id="verification-title">{t("verification.title")}</h1>
        <p>{t("verification.description")}</p>
      </header>

      {screenState.kind === "loading" ? (
        <StateCard
          id="loading"
          titleKey="verification.loading.title"
          descriptionKey="verification.loading.description"
        />
      ) : null}
      {screenState.kind === "group-required" ? (
        <StateCard
          id="group-required"
          titleKey="verification.groupRequired.title"
          descriptionKey="verification.groupRequired.description"
        >
          <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
            {t("verification.groupRequired.select")}
          </Link>
        </StateCard>
      ) : null}
      {screenState.kind === "no-groups" ? (
        <StateCard
          id="no-groups"
          titleKey="verification.noGroups.title"
          descriptionKey="verification.noGroups.description"
        />
      ) : null}
      {screenState.kind === "unavailable" ? (
        <StateCard
          id="unavailable"
          titleKey="verification.unavailable.title"
          descriptionKey={verificationErrorMessageKey(
            screenState.error,
            "verification.errors.loadUnavailable"
          )}
          role="alert"
        >
          <button
            type="button"
            data-slot="button"
            data-variant="outline"
            data-size="sm"
            onClick={() => setReloadVersion((version) => version + 1)}
          >
            {t("verification.unavailable.retry")}
          </button>
        </StateCard>
      ) : null}
      {screenState.kind === "loaded" && draft ? (
        <VerificationSettingsForm
          settings={screenState.settings}
          draft={draft}
          errors={errors}
          saving={saving}
          hasChanges={hasChanges}
          onSubmit={submit}
          onDraftChange={updateDraft}
          onRestore={restoreSetting}
        />
      ) : null}

      {feedback ? (
        <div
          data-verification-feedback
          data-tone={feedback.kind === "error" || feedback.kind === "conflict" ? "error" : "ok"}
          role={feedback.kind === "error" || feedback.kind === "conflict" ? "alert" : "status"}
          aria-atomic="true"
        >
          {t(
            feedback.kind === "saved"
              ? "verification.feedback.saved"
              : feedback.kind === "conflict"
                ? "verification.feedback.conflict"
                : feedback.messageKey
          )}
        </div>
      ) : null}
    </section>
  );
}
