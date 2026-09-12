import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { Button } from "@react-spectrum/s2/Button";

import {
  consoleApi,
  retryConsoleAccess,
  useConsoleSession
} from "../../app/session";
import { Icon } from "../../icons";
import { SettingsLimitNotice } from "../../components/SettingsLimitNotice";
import type { IconName } from "../../icons";
import type { ApiRequestError } from "../../lib/api";
import {
  capabilityFields,
  loadCapabilitySettings,
  saveCapabilitySettings,
  type CapabilityField,
  type CapabilitySettings
} from "./api";
import { CapabilityCard, SourceMeta } from "./CapabilityCard";

type CapabilityDraft = Readonly<Record<CapabilityField, boolean>>;

type CapabilitiesScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; settings: CapabilitySettings }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

type SaveFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; error: ApiRequestError }>;

type CapabilityCardDefinition = Readonly<{
  id: string;
  field: CapabilityField;
  titleKey: string;
  summaryKey: string;
  onKey: string;
  offKey: string;
  detailsPath?: string;
  detailsKey?: string;
}>;

const capabilityCardDefinitions: readonly CapabilityCardDefinition[] = [
  {
    id: "verification",
    field: "enabled",
    titleKey: "capabilities.verification.title",
    summaryKey: "capabilities.verification.summary",
    onKey: "capabilities.verification.enabled",
    offKey: "capabilities.verification.disabled",
    detailsPath: "/verification",
    detailsKey: "capabilities.verification.details"
  },
  {
    id: "antispam",
    field: "antispam_enabled",
    titleKey: "capabilities.antispam.title",
    summaryKey: "capabilities.antispam.summary",
    onKey: "capabilities.antispam.enabled",
    offKey: "capabilities.antispam.disabled",
    detailsPath: "/moderation",
    detailsKey: "capabilities.antispam.details"
  },
  {
    id: "gentoo-lookups",
    field: "gentoo_lookups_enabled",
    titleKey: "capabilities.gentooLookups.title",
    summaryKey: "capabilities.gentooLookups.summary",
    onKey: "capabilities.gentooLookups.enabled",
    offKey: "capabilities.gentooLookups.disabled"
  },
  {
    id: "linux-lookups",
    field: "linux_lookups_enabled",
    titleKey: "capabilities.linuxLookups.title",
    summaryKey: "capabilities.linuxLookups.summary",
    onKey: "capabilities.linuxLookups.enabled",
    offKey: "capabilities.linuxLookups.disabled"
  }
];

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "capabilities.errors.authenticationExpired",
  authentication_invalid: "capabilities.errors.authenticationInvalid",
  chat_access_denied: "capabilities.errors.accessDenied",
  chat_access_unavailable: "capabilities.errors.accessUnavailable",
  chat_not_found: "capabilities.errors.chatNotFound",
  csrf_invalid: "capabilities.errors.csrfInvalid",
  invalid_settings: "capabilities.errors.invalidSettings",
  settings_limit_exceeded: "capabilities.errors.settingsLimitExceeded",
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
  const [draftValues, setDraftValues] = useState<CapabilityDraft | null>(null);
  const [restoringFields, setRestoringFields] = useState<ReadonlySet<CapabilityField>>(
    new Set()
  );
  const [presetApplied, setPresetApplied] = useState(false);
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<SaveFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const saveSequenceRef = useRef(0);

  useEffect(() => {
    const scope = `${session.state}:${chatID ?? ""}:${reloadVersion}`;
    activeScopeRef.current = scope;
    saveSequenceRef.current += 1;
    setDraftValues(null);
    setPresetApplied(false);
    setRestoringFields(new Set());
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
      setDraftValues({
        enabled: result.data.enabled.value,
        antispam_enabled: result.data.antispam_enabled.value,
        gentoo_lookups_enabled: result.data.gentoo_lookups_enabled.value,
        linux_lookups_enabled: result.data.linux_lookups_enabled.value
      });
    });

    return () => {
      active = false;
    };
  }, [chatID, reloadVersion, session]);

  const settings = screenState.kind === "loaded" ? screenState.settings : null;
  const changedFields =
    settings && draftValues
      ? presetApplied
        ? capabilityFields
        : capabilityFields.filter(
            (field) =>
              restoringFields.has(field) || draftValues[field] !== settings[field].value
          )
      : [];
  const hasChanges = changedFields.length > 0;

  function toggleField(field: CapabilityField): void {
    if (
      !settings ||
      !draftValues ||
      settings[field].source === "user file" ||
      restoringFields.has(field) ||
      saving
    ) {
      return;
    }
    setDraftValues((current) =>
      current ? { ...current, [field]: !current[field] } : current
    );
    setPresetApplied(false);
    setFeedback(null);
  }

  function toggleRestore(field: CapabilityField): void {
    if (!settings || settings[field].source !== "chat override" || saving) {
      return;
    }
    setDraftValues((current) =>
      current ? { ...current, [field]: settings[field].value } : current
    );
    setRestoringFields((current) => {
      const next = new Set(current);
      if (next.has(field)) {
        next.delete(field);
      } else {
        next.add(field);
      }
      return next;
    });
    setPresetApplied(false);
    setFeedback(null);
  }

  function discardChanges(): void {
    if (!settings || saving) {
      return;
    }
    setDraftValues({
      enabled: settings.enabled.value,
      antispam_enabled: settings.antispam_enabled.value,
      gentoo_lookups_enabled: settings.gentoo_lookups_enabled.value,
      linux_lookups_enabled: settings.linux_lookups_enabled.value
    });
    setRestoringFields(new Set());
    setPresetApplied(false);
    setFeedback(null);
  }

  function applyPreset(value: boolean): void {
    setDraftValues({
      enabled: true,
      antispam_enabled: value,
      gentoo_lookups_enabled: value,
      linux_lookups_enabled: value
    });
    setRestoringFields(new Set());
    setPresetApplied(true);
    setFeedback(null);
  }

  function reloadCapabilities(): void {
    if (!retryConsoleAccess(session)) {
      setReloadVersion((version) => version + 1);
    }
  }

  async function submitSettings(): Promise<void> {
    if (!settings || !draftValues || !chatID || !hasChanges || saving) {
      return;
    }

    const changes: Partial<Record<CapabilityField, boolean | null>> = {};
    for (const field of changedFields) {
      changes[field] = restoringFields.has(field) ? null : draftValues[field];
    }

    const scope = activeScopeRef.current;
    const sequence = saveSequenceRef.current + 1;
    saveSequenceRef.current = sequence;
    setSaving(true);
    setFeedback(null);
    const result = await saveCapabilitySettings(consoleApi, chatID, settings.revision, changes);

    if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
      return;
    }
    if (result.ok) {
      setScreenState({ kind: "loaded", settings: result.data });
      setDraftValues({
        enabled: result.data.enabled.value,
        antispam_enabled: result.data.antispam_enabled.value,
        gentoo_lookups_enabled: result.data.gentoo_lookups_enabled.value,
        linux_lookups_enabled: result.data.linux_lookups_enabled.value
      });
      setRestoringFields(new Set());
      setPresetApplied(false);
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
    setFeedback({ kind: "error", error: result.error });
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
          <Button type="button" variant="primary" data-slot="button" onPress={reloadCapabilities}>
            <Icon name="refreshCw" />
            {t("capabilities.actions.retry")}
          </Button>
        </StateCard>
      ) : null}

      {settings && draftValues ? (
        <form
          data-capabilities-form
          onSubmit={(event) => {
            event.preventDefault();
            void submitSettings();
          }}
          aria-busy={saving || undefined}
        >
          <div data-capabilities-presets>
            <Button
              type="button"
              variant="secondary"
              data-slot="button"
              data-capabilities-preset="verification-only"
              onPress={() => applyPreset(false)}
            >
              <Icon name="shieldCheck" />
              {t("capabilities.presets.verificationOnly")}
            </Button>
            <Button
              type="button"
              variant="secondary"
              data-slot="button"
              data-capabilities-preset="all"
              onPress={() => applyPreset(true)}
            >
              <Icon name="circleCheck" />
              {t("capabilities.presets.all")}
            </Button>
          </div>
          <div data-capabilities-list>
            {capabilityCardDefinitions.map((definition) => {
              const field = definition.field;
              const setting = settings[field];
              const restoring = restoringFields.has(field);
              const readOnly = setting.source === "user file" || restoring || saving;
              const pending = changedFields.includes(field);
              return (
                <CapabilityCard
                  key={definition.id}
                  id={definition.id}
                  titleKey={definition.titleKey}
                  summaryKey={definition.summaryKey}
                  enabled={draftValues[field]}
                  source={setting.source}
                  onKey={definition.onKey}
                  offKey={definition.offKey}
                  detailsPath={definition.detailsPath}
                  detailsKey={definition.detailsKey}
                  groupSearch={groupSearch}
                  sourceMeta={
                    <SourceMeta
                      source={setting.source}
                      pending={pending}
                      restoring={restoring}
                      saving={saving}
                      allowRestore
                      onToggleRestore={() => toggleRestore(field)}
                    />
                  }
                  control={
                    setting.source === "user file" ? undefined : (
                      <button
                        type="button"
                        role="switch"
                        data-slot="switch"
                        aria-checked={draftValues[field]}
                        aria-disabled={readOnly ? "true" : undefined}
                        aria-labelledby={`capabilities-${definition.id}-title`}
                        aria-describedby={`capabilities-${definition.id}-summary`}
                        onClick={() => toggleField(field)}
                      />
                    )
                  }
                />
              );
            })}
          </div>

          {hasChanges ? (
            <aside data-slot="card" data-capabilities-savebar aria-label={t("capabilities.save.label")}>
              <span aria-live="polite">{t("capabilities.save.unsaved")}</span>
              <span data-capabilities-save-actions>
                <Button
                  type="button"
                  variant="secondary"
                  data-slot="button"
                  aria-disabled={saving ? "true" : undefined}
                  onPress={discardChanges}
                >
                  <Icon name="trash2" />
                  {t("capabilities.actions.discard")}
                </Button>
                <Button
                  type="submit"
                  variant="accent"
                  data-slot="button"
                  isPending={saving}
                  aria-disabled={saving ? "true" : undefined}
                >
                  <Icon name="save" />
                  {t(saving ? "capabilities.actions.saving" : "capabilities.actions.save")}
                </Button>
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
          <div>
            <Icon name={feedback.kind === "saved" ? "circleCheck" : "circleAlert"} />
            {feedback.kind === "error" &&
            feedback.error.kind === "api" &&
            feedback.error.code === "settings_limit_exceeded" ? (
              <SettingsLimitNotice
                error={feedback.error}
                messageKey="capabilities.errors.settingsLimitExceeded"
              />
            ) : (
              t(
                feedback.kind === "saved"
                  ? "capabilities.feedback.saved"
                  : feedback.kind === "conflict"
                    ? "capabilities.feedback.conflict"
                    : capabilityErrorMessageKey(feedback.error, "capabilities.errors.saveUnavailable")
              )
            )}
          </div>
          {feedback.kind === "conflict" ||
          (feedback.kind === "error" &&
            (feedback.error.kind === "network" ||
              (feedback.error.kind === "api" && feedback.error.code === "invalid_settings"))) ? (
            <Button type="button" variant="secondary" data-slot="button" onPress={reloadCapabilities}>
              <Icon name="refreshCw" />
              {t("capabilities.actions.reload")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
