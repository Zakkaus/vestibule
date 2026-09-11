import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { Icon } from "../../icons";

import { canViewOwner, consoleApi, useConsoleSession } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import {
  loadOwnerLimits,
  ownerLimitFields,
  saveOwnerLimits,
  type OwnerLimitChanges,
  type OwnerLimitField,
  type OwnerLimits,
  type OwnerLimitsResponse
} from "./api";


type ScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; response: OwnerLimitsResponse }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

type Feedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "error"; error: ApiRequestError }>;

function errorKey(error: ApiRequestError): string {
  if (error.kind === "network") return "owner.errors.network";
  if (error.kind !== "api") return "owner.errors.unavailable";
  switch (error.code) {
    case "csrf_invalid":
      return "owner.errors.csrfInvalid";
    case "invalid_request":
      return "owner.errors.invalidRequest";
    case "owner_required":
      return "owner.errors.ownerRequired";
    case "settings_conflict":
      return "owner.errors.conflict";
    case "settings_unavailable":
      return "owner.errors.settingsUnavailable";
    default:
      return "owner.errors.unavailable";
  }
}

function changedLimits(
  original: OwnerLimits,
  draft: OwnerLimits
): OwnerLimitChanges {
  const changes: OwnerLimitChanges = {};
  for (const field of ownerLimitFields) {
    if (original[field] !== draft[field]) changes[field] = draft[field];
  }
  return changes;
}

function StateCard({
  titleKey,
  descriptionKey,
  role
}: Readonly<{ titleKey: string; descriptionKey: string; role?: "alert" }>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-owner-state-card role={role}>
      <h2>{t(titleKey)}</h2>
      <p>{t(descriptionKey)}</p>
    </section>
  );
}

export function OwnerLimitsScreen() {
  const { t } = useTranslation();
  const session = useConsoleSession();
  const isOwner = canViewOwner(session);
  const [screenState, setScreenState] = useState<ScreenState>({ kind: "loading" });
  const [draft, setDraft] = useState<OwnerLimits | null>(null);
  const [feedback, setFeedback] = useState<Feedback | null>(null);
  const [saving, setSaving] = useState(false);
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => {
    let active = true;
    setFeedback(null);
    setDraft(null);
    if (!isOwner) {
      setScreenState({ kind: "loading" });
      return () => {
        active = false;
      };
    }

    setScreenState({ kind: "loading" });
    void loadOwnerLimits(consoleApi).then((result) => {
      if (!active) return;
      if (!result.ok) {
        setScreenState({ kind: "unavailable", error: result.error });
        return;
      }
      setScreenState({ kind: "loaded", response: result.data });
      setDraft({ ...result.data.limits });
    });
    return () => {
      active = false;
    };
  }, [isOwner, reloadVersion]);

  if (!isOwner) return null;

  const loaded = screenState.kind === "loaded" ? screenState.response : null;
  const changes = loaded && draft ? changedLimits(loaded.limits, draft) : {};
  const hasChanges = Object.keys(changes).length > 0;
  const canReload =
    feedback?.kind === "error" &&
    feedback.error.kind === "api" &&
    feedback.error.code === "settings_conflict";

  async function save(): Promise<void> {
    if (!loaded || !draft || !hasChanges || saving) return;
    setSaving(true);
    setFeedback(null);
    const result = await saveOwnerLimits(consoleApi, loaded.revision, changes);
    if (result.ok) {
      setScreenState({ kind: "loaded", response: result.data });
      setDraft({ ...result.data.limits });
      setFeedback({ kind: "saved" });
    } else {
      setFeedback({ kind: "error", error: result.error });
    }
    setSaving(false);
  }

  return (
    <section
      data-owner-page
      aria-busy={screenState.kind === "loading" || saving ? true : undefined}
      aria-labelledby="owner-title"
    >
      <header data-page-heading>
        <h1 id="owner-title" data-state-heading><Icon name="settings" />{t("owner.title")}</h1>
        <p>{t("owner.description")}</p>
      </header>

      {screenState.kind === "loading" ? (
        <StateCard titleKey="owner.loading.title" descriptionKey="owner.loading.description" />
      ) : null}
      {screenState.kind === "unavailable" ? (
        <StateCard
          titleKey="owner.unavailable.title"
          descriptionKey={errorKey(screenState.error)}
          role="alert"
        />
      ) : null}

      {loaded && draft ? (
        <form
          data-owner-limits-form
          onSubmit={(event) => {
            event.preventDefault();
            void save();
          }}
        >
          <div data-slot="card" data-owner-limits-fields>
            {ownerLimitFields.map((field: OwnerLimitField) => {
              const inputID = `owner-limit-${field}`;
              const readOnly = saving;
              return (
                <div
                  key={field}
                  data-slot="setting"
                  data-read-only={readOnly || undefined}
                >
                  <div data-setting-copy>
                    <div data-setting-heading>
                      <label htmlFor={inputID}>{t(`owner.fields.${field}`)}</label>
                    </div>
                  </div>
                  <div data-setting-control>
                    <input
                      data-slot="input"
                      data-owner-limit-input={field}
                      id={inputID}
                      type="number"
                      min={0}
                      step={1}
                      value={draft[field]}
                      readOnly={readOnly}
                      onChange={(event) => {
                        const value = Number(event.currentTarget.value);
                        if (Number.isSafeInteger(value) && value >= 0) {
                          setDraft((current) => (current ? { ...current, [field]: value } : current));
                          setFeedback(null);
                        }
                      }}
                    />
                  </div>
                </div>
              );
            })}
          </div>

          <aside
            data-owner-limits-savebar
            data-save-state={saving ? "submitting" : "idle"}
            aria-label={t("owner.actions.save")}
          >
            <span aria-live="polite">
              {hasChanges ? t("owner.actions.unsaved") : null}
            </span>
            <button
              type="submit"
              data-slot="button"
              data-variant="primary"
              data-size="sm"
              data-owner-limits-save
              disabled={!hasChanges || saving}
            >
              <Icon name={saving ? "loaderCircle" : "save"} />
              {t(saving ? "owner.actions.saving" : "owner.actions.save")}
            </button>
          </aside>

          {loaded.violations.length > 0 ? (
            <section data-slot="card" data-owner-limit-violations aria-labelledby="owner-violations-title">
              <h2 id="owner-violations-title">{t("owner.violations.title")}</h2>
              <ul>
                {loaded.violations.map((violation) => (
                  <li
                    key={`${violation.chat_id}:${violation.field}`}
                    data-owner-limit-violation={`${violation.chat_id}:${violation.field}`}
                  >
                    {t("owner.violations.row", {
                      chatId: violation.chat_id,
                      field: t(`owner.fields.${violation.field}`),
                      value: violation.value,
                      limit: violation.limit
                    })}
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          {feedback ? (
            <div
              data-owner-limits-feedback
              data-status={feedback.kind === "saved" ? "ok" : "error"}
              role={feedback.kind === "error" ? "alert" : "status"}
            >
              <span>
                {feedback.kind === "saved" ? t("owner.feedback.saved") : t(errorKey(feedback.error))}
              </span>
              {canReload ? (
                <button
                  type="button"
                  data-slot="button"
                  data-variant="outline"
                  data-size="sm"
                  onClick={() => setReloadVersion((version) => version + 1)}
                >
                  <Icon name="refreshCw" />
                  {t("owner.actions.retry")}
                </button>
              ) : null}
            </div>
          ) : null}
        </form>
      ) : null}

      {screenState.kind === "unavailable" ? (
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-size="sm"
          onClick={() => setReloadVersion((version) => version + 1)}
        >
          <Icon name="refreshCw" />
          {t("owner.actions.retry")}
        </button>
      ) : null}
    </section>
  );
}
