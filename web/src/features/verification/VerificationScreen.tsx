import { PageHeader } from "../../components/PageHeader";
import { useDraftOwner, useScopeChange } from "../../app/drafts";
import { Feedback, writeOutcomeUnknown } from "../../components/feedback";
import { changedDraftFields, reapplyDraft, VerificationConflict } from "./VerificationConflict";
import { Button } from "@react-spectrum/s2/Button";
import { Text } from "@react-spectrum/s2";
import { ProgressCircle } from "@react-spectrum/s2/ProgressCircle";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import {
  consoleApi,
  retryConsoleAccess,
  useConsoleSession
} from "../../app/session";
import { Icon, type IconName } from "../../icons";
import { SettingsLimitNotice } from "../../components/SettingsLimitNotice";
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
  | Readonly<{ kind: "error"; error: ApiRequestError; unknown?: boolean; refetch?: boolean }>;

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "verification.errors.authenticationExpired",
  authentication_invalid: "verification.errors.authenticationInvalid",
  chat_access_denied: "verification.errors.accessDenied",
  chat_access_unavailable: "verification.errors.accessUnavailable",
  chat_not_found: "verification.errors.chatNotFound",
  csrf_invalid: "verification.errors.csrfInvalid",
  invalid_settings: "verification.errors.invalidSettings",
  settings_limit_exceeded: "verification.errors.settingsLimitExceeded",
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

  return (
    <section
      data-slot="card"
      data-verification-state-card={id}
      role={role}
      aria-labelledby={`verification-${id}-title`}
    >
      <h2 id={`verification-${id}-title`} data-state-heading>
        <Icon name={icon} />
        {t(titleKey)}
      </h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

export function VerificationScreen() {
  const { t } = useTranslation();
  const size = "M";
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
  const [touched, setTouched] = useState<ReadonlySet<VerificationSettingField>>(new Set());
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<SaveFeedback | null>(null);
  const [conflict, setConflict] = useState<VerificationSettings | null>(null);
  const requestScopeChange = useScopeChange();
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
    setTouched(new Set());
    setFeedback(null);
    setConflict(null);
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
  const errors: FieldErrors = attemptedSave ? validation.errors : Object.fromEntries(
    Object.entries(validation.errors).filter(([field]) => touched.has(field as VerificationSettingField))
  );
  const dirtyCount = screenState.kind === "loaded" && draft ? changedDraftFields(screenState.settings, draft, restored).length : 0;
  function discardDraft(): void {
    if (screenState.kind !== "loaded") return;
    const settings = conflict ?? screenState.settings;
    setScreenState({ kind: "loaded", settings });
    setDraft(settingsDraft(settings));
    setRestored(new Set());
    setAttemptedSave(false);
    setTouched(new Set());
    setFeedback(null);
    setConflict(null);
  }
  useDraftOwner(hasChanges, saving, discardDraft);

  function reapplyChanges(): void {
    if (!conflict || screenState.kind !== "loaded" || !draft) return;
    setDraft(reapplyDraft(screenState.settings, draft, conflict, restored));
    setScreenState({ kind: "loaded", settings: conflict });
    setConflict(null);
    setFeedback(null);
  }

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
    if (screenState.kind !== "loaded" || !draft || !chatID || saving || conflict) {
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
        setConflict(currentSettings.data);
        setSaving(false);
        return;
      }
      setFeedback({ kind: "error", error: currentSettings.error, refetch: true });
      setSaving(false);
      return;
    }
    if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
      setScreenState({ kind: "unavailable", error: result.error });
      setSaving(false);
      return;
    }
    setFeedback({ kind: "error", error: result.error, unknown: writeOutcomeUnknown(result.error), refetch: writeOutcomeUnknown(result.error) });
    setSaving(false);
  }

  function reloadVerification(): void {
    requestScopeChange(() => {
      if (!retryConsoleAccess(session)) setReloadVersion((version) => version + 1);
    });
  }

  async function refetchOutcome(): Promise<void> {
    if (!chatID || saving) return;
    setSaving(true);
    const scope = activeScopeRef.current;
    const latest = await loadVerificationSettings(consoleApi, chatID);
    if (scope !== activeScopeRef.current) return;
    setSaving(false);
    if (!latest.ok) {
      setFeedback({ kind: "error", error: latest.error, refetch: true });
      return;
    }
    // A read does not prove an unacknowledged write finished. Review this revision
    // without dropping the local delta or resending the write.
    setConflict(latest.data);
    setFeedback(null);
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
      <PageHeader ><h1 id="verification-title">
        <Icon name="shieldCheck" />
        {t("verification.title")}
      </h1>
      <p>{t("verification.description")}</p></PageHeader>

      {conflict && screenState.kind === "loaded" && draft ? (
        <VerificationConflict baseline={screenState.settings} draft={draft} latest={conflict}
          restored={restored} onDiscard={discardDraft} onReapply={reapplyChanges} />
      ) : null}
      {feedback ? (
        <Feedback data-verification-feedback focusFailure={feedback.kind === "error"} level={feedback.kind === "saved" ? "positive" : "negative"}
          message={t(feedback.kind === "saved" ? "verification.feedback.saved" : verificationErrorMessageKey(feedback.error, "verification.errors.saveUnavailable"))}
          unknown={feedback.kind === "error" && feedback.unknown}
          onRefetch={feedback.kind === "error" && feedback.refetch ? () => { void refetchOutcome(); } : undefined}>
          {feedback.kind === "error" && feedback.error.kind === "api" && feedback.error.code === "settings_limit_exceeded"
            ? <SettingsLimitNotice error={feedback.error} messageKey="verification.errors.settingsLimitExceeded" /> : undefined}
        </Feedback>
      ) : null}
      {screenState.kind === "loading" ? <div><ProgressCircle aria-label={t("verification.loading.title")} isIndeterminate /><p>{t("verification.loading.description")}</p></div> : null}
      {screenState.kind === "group-required" ? (
        <StateCard
          id="group-required"
          icon="usersRound"
          titleKey="verification.groupRequired.title"
          descriptionKey="verification.groupRequired.description"
        >
          <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
            <Icon name="usersRound" />
            {t("verification.groupRequired.select")}
          </Link>
        </StateCard>
      ) : null}
      {screenState.kind === "no-groups" ? (
        <StateCard
          id="no-groups"
          icon="usersRound"
          titleKey="verification.noGroups.title"
          descriptionKey="verification.noGroups.description"
        />
      ) : null}
      {screenState.kind === "unavailable" ? (
        <StateCard
          id="unavailable"
          icon="circleAlert"
          titleKey="verification.unavailable.title"
          descriptionKey={verificationErrorMessageKey(
            screenState.error,
            "verification.errors.loadUnavailable"
          )}
          role="alert"
        >
          <Button
            type="button"
            variant="primary"
            size={size}
            data-slot="button"
            onPress={reloadVerification}
          >
            <Icon name="refreshCw" />
            <Text styles={style({ whiteSpace: "nowrap" })}>{t("verification.unavailable.retry")}</Text>
          </Button>
        </StateCard>
      ) : null}
      {screenState.kind === "loaded" && draft ? (
        <VerificationSettingsForm
          settings={screenState.settings}
          draft={draft}
          errors={errors}
          saving={saving}
          dirtyCount={dirtyCount}
          onDiscard={discardDraft}
          saveBlocked={conflict !== null || !validation.values}
          onSubmit={submit}
          onDraftChange={updateDraft}
          onFieldBlur={(field) => setTouched((current) => new Set(current).add(field))}
          onRestore={restoreSetting}
        />
      ) : null}

    </section>
  );
}
