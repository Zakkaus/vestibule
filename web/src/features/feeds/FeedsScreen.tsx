import { Button, Text } from "@react-spectrum/s2/Button";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { consoleApi, retryConsoleAccess, useConsoleSession } from "../../app/session";
import { useConsoleSize } from "../../components/ConsoleProvider";
import { StatusBadge } from "../../components/StatusBadge";
import { Icon, type IconName } from "../../icons";
import { ApiError, type ApiRequestError } from "../../lib/api";
import { groupName } from "../../lib/chatNames";
import {
  loadFeedSettings,
  loadProcessSettings,
  saveFeedSettings,
  type FeedSettings,
  type FeedSettingSource,
  type OverlayConfig,
  type ProcessSettings,
  type Setting
} from "./api";
import { FeedSettingsForm } from "./FeedSettingsForm";
import {
  factoryDraft,
  hasDraftChanges,
  settingsDraft,
  validateDraft,
  type FeedDraft,
  type FeedFieldError
} from "./form";

type FeedsScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; chatID: string; settings: FeedSettings; process?: ProcessSettings }>
  | Readonly<{ kind: "access-denied" }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

type SaveFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "conflict" }>
  | Readonly<{ kind: "error"; error: ApiRequestError; messageKey: string }>;


const sourceMessageKeys: Readonly<Record<FeedSettingSource, string>> = {
  "factory default": "feeds.source.factoryDefault",
  "user file": "feeds.source.userFile",
  "chat override": "feeds.source.chatOverride"
};

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "feeds.errors.authenticationExpired",
  authentication_invalid: "feeds.errors.authenticationInvalid",
  chat_access_denied: "feeds.errors.accessDenied",
  csrf_invalid: "feeds.errors.csrfInvalid",
  chat_not_found: "feeds.errors.loadUnavailable",
  process_settings_unavailable: "feeds.errors.settingsUnavailable",
  settings_unavailable: "feeds.errors.settingsUnavailable"
};

const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_not_found: true
};

const fieldMessageKeys: Readonly<Record<string, string>> = {
  required_field: "feeds.validation.required",
  invalid_interval: "feeds.validation.invalidInterval",
  invalid_language: "feeds.validation.invalidLanguage",
  invalid_repository: "feeds.validation.invalidRepository",
  duplicate_repository: "feeds.validation.duplicateRepository",
  invalid_revision: "feeds.validation.invalidRevision"
};

function errorMessageKey(error: ApiRequestError, fallback = "feeds.errors.loadUnavailable"): string {
  if (error.kind === "network") return "feeds.errors.network";
  if (error.kind === "api") return errorMessageKeys[error.code] ?? fallback;
  return "feeds.errors.invalidResponse";
}

function serverFieldMessageKey(name: string, code: string): string | undefined {
  const messageKey = fieldMessageKeys[code];
  if (!messageKey) return undefined;
  if (name === "expected_revision") return code === "invalid_revision" ? messageKey : undefined;
  if (name === "lang") return code === "invalid_language" || code === "required_field" ? messageKey : undefined;
  if (name === "interval_seconds") return code === "invalid_interval" || code === "required_field" ? messageKey : undefined;
  if (name === "bugs" || name === "news" || name === "bug_product" || name === "bug_component" || name === "silent_bugs") {
    return code === "required_field" ? messageKey : undefined;
  }
  const repository = /^github_repos\[(\d+)\](?:\.(repo|branch|issues|pulls))?$/.exec(name);
  if (!repository) return undefined;
  if (repository[2] === "repo") return code === "invalid_repository" || code === "required_field" ? messageKey : undefined;
  if (repository[2] === undefined) return code === "duplicate_repository" || code === "required_field" ? messageKey : undefined;
  return code === "required_field" ? messageKey : undefined;
}

function mapServerErrors(error: ApiRequestError): Readonly<{
  fields: FeedFieldError;
  saveBarErrorKey?: string;
  messageKey: string;
}> {
  if (error.kind !== "api") {
    return { fields: {}, messageKey: errorMessageKey(error, "feeds.errors.saveUnavailable") };
  }
  if (error.code !== "invalid_request") {
    return { fields: {}, messageKey: errorMessageKey(error, "feeds.errors.saveUnavailable") };
  }
  const fields: Record<string, string> = {};
  let saveBarErrorKey: string | undefined;
  let unknown = error.fields.length === 0;
  for (const field of error.fields) {
    const messageKey = serverFieldMessageKey(field.name, field.code);
    if (!messageKey) {
      unknown = true;
      continue;
    }
    if (field.name === "expected_revision") {
      saveBarErrorKey = messageKey;
    } else {
      fields[field.name] = messageKey;
    }
  }
  const messageKey = saveBarErrorKey ?? (unknown ? "feeds.errors.invalidRequest" : "feeds.errors.reviewFields");
  return { fields, saveBarErrorKey, messageKey };
}

function SourceBadge({ source }: Readonly<{ source: FeedSettingSource }>) {
  const { t } = useTranslation();
  return <StatusBadge tone="neutral">{t("feeds.source.value", { source: t(sourceMessageKeys[source]) })}</StatusBadge>;
}

function StateCard({ id, titleKey, descriptionKey, iconName, role, live, children }: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  iconName: IconName;
  role?: "alert";
  live?: "polite";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-feeds-state-card={id} role={role} aria-live={live} aria-labelledby={`feeds-${id}-title`}>
      <h2 id={`feeds-${id}-title`}><span data-state-heading><Icon name={iconName} />{t(titleKey)}</span></h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

function SectionHeading({ id, titleKey, descriptionKey, source }: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  source: FeedSettingSource;
}>) {
  const { t } = useTranslation();
  return (
    <header data-feeds-section-heading>
      <div data-feeds-section-copy><h2 id={id}>{t(titleKey)}</h2><p>{t(descriptionKey)}</p></div>
      <SourceBadge source={source} />
    </header>
  );
}

function NewsURLSection({ settings }: Readonly<{ settings: ProcessSettings["newsURL"] }>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-feeds-section="news-url" data-process-setting-source={settings.source} aria-labelledby="feeds-news-url-title">
      <SectionHeading id="feeds-news-url-title" titleKey="feeds.newsURL.title" descriptionKey="feeds.newsURL.description" source={settings.source as FeedSettingSource} />
      <div className="surface-raised" data-news-url-value>
        {settings.value ? <code>{settings.value}</code> : <span data-state-heading><Icon name="inbox" />{t("feeds.newsURL.empty")}</span>}
      </div>
    </section>
  );
}

function OverlayItem({ overlay, number }: Readonly<{ overlay: OverlayConfig; number: number }>) {
  const { t } = useTranslation();
  return (
    <article className="surface-raised" data-overlay-item aria-labelledby={`overlay-item-${number}-title`}>
      <h3 id={`overlay-item-${number}-title`}>{overlay.name || overlay.repo}</h3>
      <dl data-overlay-values>
        <div data-overlay-value><dt>{t("feeds.overlays.repository")}</dt><dd><code>{overlay.repo}</code></dd></div>
        <div data-overlay-value><dt>{t("feeds.overlays.branch")}</dt><dd><code>{overlay.branch || t("feeds.overlays.defaultBranch")}</code></dd></div>
      </dl>
    </article>
  );
}

function OverlaySection({ settings }: Readonly<{ settings: ProcessSettings["overlays"] }>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-feeds-section="overlays" data-process-setting-source={settings.source} aria-labelledby="feeds-overlays-title">
      <SectionHeading id="feeds-overlays-title" titleKey="feeds.overlays.title" descriptionKey="feeds.overlays.description" source={settings.source as FeedSettingSource} />
      {settings.value.length === 0 ? (
        <div className="surface-raised" data-overlays-empty><div data-state-heading><Icon name="inbox" /><strong>{t("feeds.overlays.emptyTitle")}</strong></div><p>{t("feeds.overlays.emptyDescription")}</p></div>
      ) : <div data-overlay-list>{settings.value.map((overlay, index) => <OverlayItem key={`${overlay.name}:${index}`} overlay={overlay} number={index + 1} />)}</div>}
    </section>
  );
}

export function FeedsScreen() {
  const { t } = useTranslation();
  const size = useConsoleSize("L");
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const [screenState, setScreenState] = useState<FeedsScreenState>({ kind: "loading" });
  const [draft, setDraft] = useState<FeedDraft | null>(null);
  const [attemptedSave, setAttemptedSave] = useState(false);
  const [serverErrors, setServerErrors] = useState<FeedFieldError>({});
  const [saveBarErrorKey, setSaveBarErrorKey] = useState<string | undefined>();
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<SaveFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const saveSequenceRef = useRef(0);

  const selectedGroupID = searchParams.get("group");
  const selectedChatID = selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID)
    ? selectedGroupID
    : session.state === "ready" ? session.chats[0]?.id : undefined;

  useEffect(() => {
    const scope = `${session.state}:${selectedChatID ?? ""}:${reloadVersion}`;
    activeScopeRef.current = scope;
    saveSequenceRef.current += 1;
    setSaving(false);
    setDraft(null);
    setAttemptedSave(false);
    setServerErrors({});
    setSaveBarErrorKey(undefined);
    setFeedback(null);
    let active = true;

    if (session.state === "loading" || session.state === "checking-groups") {
      setScreenState({ kind: "loading" });
      return () => { active = false; };
    }
    if (session.state === "blocked" || session.state === "groups-unavailable") {
      setScreenState({ kind: "unavailable", error: session.error });
      return () => { active = false; };
    }
    if (!selectedChatID) {
      setScreenState({ kind: "unavailable", error: new ApiError("chat_not_found", 404) });
      return () => { active = false; };
    }

    setScreenState({ kind: "loading" });
    void Promise.all([loadFeedSettings(consoleApi, selectedChatID), loadProcessSettings(consoleApi)]).then(([feedResult, processResult]) => {
      if (!active || activeScopeRef.current !== scope) return;
      if (!feedResult.ok) {
        setScreenState(feedResult.error.kind === "api" && feedResult.error.code === "chat_access_denied"
          ? { kind: "access-denied" }
          : { kind: "unavailable", error: feedResult.error });
        return;
      }
      setScreenState({ kind: "loaded", chatID: selectedChatID, settings: feedResult.data, process: processResult.ok ? processResult.data : undefined });
      setDraft(settingsDraft(feedResult.data));
    });
    return () => { active = false; };
  }, [reloadVersion, selectedChatID, session]);

  const validation = draft ? validateDraft(draft) : { errors: {} };
  const hasChanges = screenState.kind === "loaded" && draft ? hasDraftChanges(screenState.settings, draft) : false;
  const errors: FeedFieldError = attemptedSave ? { ...validation.errors, ...serverErrors } : serverErrors;

  function updateDraft(next: FeedDraft): void {
    if (saving) return;
    setDraft(next);
    setServerErrors({});
    setSaveBarErrorKey(undefined);
    setFeedback(null);
  }

  function restoreFactory(): void {
    if (!saving) updateDraft(factoryDraft());
  }

  async function submitFeedSettings(): Promise<void> {
    if (screenState.kind !== "loaded" || !draft || !selectedChatID || saving) return;
    setAttemptedSave(true);
    if (!validation.values || !hasChanges) return;
    const scope = activeScopeRef.current;
    const sequence = saveSequenceRef.current + 1;
    saveSequenceRef.current = sequence;
    setSaving(true);
    setServerErrors({});
    setSaveBarErrorKey(undefined);
    setFeedback(null);
    const result = await saveFeedSettings(consoleApi, selectedChatID, screenState.settings.revision, validation.values);
    if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) return;
    if (result.ok) {
      setScreenState({ kind: "loaded", chatID: selectedChatID, settings: result.data, process: screenState.process });
      setDraft(settingsDraft(result.data));
      setAttemptedSave(false);
      setSaving(false);
      setFeedback({ kind: "saved" });
      return;
    }
    if (result.error.kind === "api" && result.error.code === "settings_conflict") {
      const latest = await loadFeedSettings(consoleApi, selectedChatID);
      if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) return;
      if (latest.ok) {
        setScreenState((current) => current.kind === "loaded" ? { ...current, settings: latest.data } : current);
        setDraft(settingsDraft(latest.data));
        setAttemptedSave(false);
        setServerErrors({});
        setSaveBarErrorKey(undefined);
        setSaving(false);
        setFeedback({ kind: "conflict" });
        return;
      }
      setScreenState({ kind: "unavailable", error: latest.error });
      setSaving(false);
      return;
    }
    if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
      setScreenState({ kind: "unavailable", error: result.error });
      setSaving(false);
      return;
    }
    const mapped = mapServerErrors(result.error);
    setServerErrors(mapped.fields);
    setSaveBarErrorKey(mapped.saveBarErrorKey);
    setFeedback({ kind: "error", error: result.error, messageKey: mapped.messageKey });
    setSaving(false);
  }

  function reloadFeeds(): void {
    if (!retryConsoleAccess(session)) setReloadVersion((version) => version + 1);
  }

  const title = session.state === "ready" && screenState.kind === "loaded"
    ? session.chats.find((chat) => chat.id === screenState.chatID)?.title
    : undefined;

  return (
    <section data-feeds-page data-feeds-state={screenState.kind} aria-busy={screenState.kind === "loading" || saving || undefined} aria-labelledby="feeds-title">
      <header data-page-heading><h1 id="feeds-title"><Icon name="rss" />{t("feeds.title")}</h1><p>{t("feeds.description")}</p></header>
      {screenState.kind === "loading" ? <StateCard id="loading" titleKey="feeds.loading.title" descriptionKey="feeds.loading.description" live="polite" iconName="loaderCircle" /> : null}
      {screenState.kind === "access-denied" ? <StateCard id="access-denied" titleKey="feeds.accessDenied.title" descriptionKey="feeds.accessDenied.description" role="alert" iconName="circleAlert" /> : null}
      {screenState.kind === "unavailable" ? (
        <StateCard id="unavailable" titleKey="feeds.unavailable.title" descriptionKey={errorMessageKey(screenState.error)} role="alert" iconName="circleAlert">
          <Button type="button" variant="primary" size={size} data-slot="button" onPress={reloadFeeds}><Icon name="refreshCw" /><Text>{t("feeds.unavailable.retry")}</Text></Button>
        </StateCard>
      ) : null}
      {screenState.kind === "loaded" && draft ? (
        <div data-feeds-content>
          <FeedSettingsForm
            settings={screenState.settings}
            draft={draft}
            errors={errors}
            saveBarErrorKey={saveBarErrorKey}
            saving={saving}
            hasChanges={hasChanges}
            onSubmit={(event) => { event.preventDefault(); void submitFeedSettings(); }}
            onDraftChange={updateDraft}
            onRestoreFactory={restoreFactory}
          />
          {screenState.process ? <><NewsURLSection settings={screenState.process.newsURL} /><OverlaySection settings={screenState.process.overlays} /></> : null}
          {feedback ? (
            <div data-feeds-feedback data-tone={feedback.kind === "saved" ? "ok" : "error"} role={feedback.kind === "saved" ? "status" : "alert"} aria-atomic="true">
              <Icon name={feedback.kind === "saved" ? "circleCheck" : "circleAlert"} />
              {t(feedback.kind === "saved" ? "feeds.feedback.saved" : feedback.kind === "conflict" ? "feeds.feedback.conflict" : feedback.messageKey)}
              {feedback.kind === "error" && feedback.error.kind === "network" ? <Button type="button" variant="secondary" size={size} data-slot="button" onPress={reloadFeeds}><Icon name="refreshCw" /><Text>{t("feeds.actions.reload")}</Text></Button> : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}
