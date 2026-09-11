import { Button } from "@react-spectrum/s2/Button";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { consoleApi, useConsoleSession } from "../../app/session";
import { Icon } from "../../icons";
import type { ApiRequestError } from "../../lib/api";
import { DiagnosticsCard } from "./DiagnosticsCard";
import {
  loadDailyStatus,
  saveDailyStatus,
  type DiagnosticsDaily
} from "./api";

type DailyState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; daily: DiagnosticsDaily }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

type DailyFeedback =
  | Readonly<{ kind: "saved" }>
  | Readonly<{ kind: "error"; error: ApiRequestError }>;

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "diagnostics.errors.authenticationExpired",
  authentication_invalid: "diagnostics.errors.authenticationInvalid",
  csrf_invalid: "diagnostics.errors.csrfInvalid",
  diagnostics_access_denied: "diagnostics.errors.accessDenied",
  diagnostics_unavailable: "diagnostics.errors.unavailable",
  invalid_json: "diagnostics.errors.invalidRequest",
  invalid_request: "diagnostics.errors.invalidRequest",
  json_required: "diagnostics.errors.invalidRequest"
};
const nonRetryableErrorCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  diagnostics_access_denied: true
};

function dailyErrorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "diagnostics.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? fallback;
  }
  return "diagnostics.errors.invalidResponse";
}

function DailyUnavailable({
  error,
  onRetry
}: Readonly<{ error: ApiRequestError; onRetry: () => void }>) {
  const { t } = useTranslation();
  const accessDenied = error.kind === "api" && error.code === "diagnostics_access_denied";
  const canRetry = error.kind !== "api" || nonRetryableErrorCodes[error.code] !== true;
  return (
    <section
      data-slot="card"
      data-diagnostics-state-card="unavailable"
      data-diagnostics-daily-state-card={accessDenied ? "access-denied" : "unavailable"}
      role="alert"
      aria-labelledby="diagnostics-daily-error-title"
    >
      <h2 id="diagnostics-daily-error-title">
        <span data-state-heading>
          <Icon name="circleAlert" />
          {t(accessDenied ? "diagnostics.daily.accessDenied.title" : "diagnostics.daily.unavailable.title")}
        </span>
      </h2>
      <p>
        {t(
          accessDenied
            ? "diagnostics.daily.accessDenied.description"
            : dailyErrorMessageKey(error, "diagnostics.daily.unavailable.description")
        )}
      </p>
      {/* The button's own width beats a stylesheet rule, so the narrow-screen
          full width is declared here rather than in app.css. */}
      {canRetry ? (
        <Button
          variant="primary"
          size="S"
          data-slot="button"
          data-size="sm"
          styles={style({ width: { default: "fit", "@media (max-width: 48rem)": "full" } })}
          onPress={onRetry}
        >
          <Icon name="refreshCw" />
          {t("diagnostics.daily.actions.retry")}
        </Button>
      ) : null}
    </section>
  );
}

function DailyFeedbackNotice({ feedback }: Readonly<{ feedback: DailyFeedback }>) {
  const { t } = useTranslation();
  const messageKey =
    feedback.kind === "saved"
      ? "diagnostics.daily.feedback.saved"
      : dailyErrorMessageKey(feedback.error, "diagnostics.daily.feedback.error");
  return (
    <div
      data-slot="badge"
      data-diagnostics-daily-feedback={feedback.kind}
      data-status={feedback.kind === "saved" ? "ok" : "error"}
      role={feedback.kind === "saved" ? "status" : "alert"}
      aria-atomic="true"
    >
      <Icon name={feedback.kind === "saved" ? "circleCheck" : "circleAlert"} />
      <span>{t(messageKey)}</span>
    </div>
  );
}

function useDailyStatus() {
  const session = useConsoleSession();
  const [state, setState] = useState<DailyState>({ kind: "loading" });
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<DailyFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const activeScopeRef = useRef("");
  const saveSequenceRef = useRef(0);

  useEffect(() => {
    const scope = `${session.state}:${reloadVersion}`;
    activeScopeRef.current = scope;
    saveSequenceRef.current += 1;
    setSaving(false);
    setFeedback(null);
    let active = true;

    if (session.state === "loading" || session.state === "checking-groups") {
      setState({ kind: "loading" });
      return () => {
        active = false;
      };
    }
    if (session.state === "blocked") {
      setState({ kind: "unavailable", error: session.error });
      return () => {
        active = false;
      };
    }

    setState({ kind: "loading" });
    void loadDailyStatus(consoleApi).then((result) => {
      if (active && activeScopeRef.current === scope) {
        setState(result.ok ? { kind: "loaded", daily: result.data } : { kind: "unavailable", error: result.error });
      }
    });

    return () => {
      active = false;
    };
  }, [reloadVersion, session]);

  async function toggleEnabled(): Promise<void> {
    if (state.kind !== "loaded" || saving) {
      return;
    }

    const scope = activeScopeRef.current;
    const sequence = saveSequenceRef.current + 1;
    saveSequenceRef.current = sequence;
    setSaving(true);
    setFeedback(null);
    const result = await saveDailyStatus(consoleApi, !state.daily.enabled);
    if (activeScopeRef.current !== scope || saveSequenceRef.current !== sequence) {
      return;
    }
    if (result.ok) {
      setState({ kind: "loaded", daily: result.data });
      setFeedback({ kind: "saved" });
    } else if (result.error.kind === "api" && nonRetryableErrorCodes[result.error.code] === true) {
      setState({ kind: "unavailable", error: result.error });
    } else {
      setFeedback({ kind: "error", error: result.error });
    }
    setSaving(false);
  }

  return { state, saving, feedback, toggleEnabled, retry: () => setReloadVersion((version) => version + 1) };
}

export function DailyStatusSection() {
  const { t } = useTranslation();
  const { state, saving, feedback, toggleEnabled, retry } = useDailyStatus();

  if (state.kind === "loading") {
    return (
      <DiagnosticsCard
        id="daily"
        titleKey="diagnostics.daily.title"
        descriptionKey="diagnostics.daily.description"
      >
        <p data-diagnostics-daily-loading aria-live="polite">
          {t("diagnostics.daily.loading")}
        </p>
      </DiagnosticsCard>
    );
  }
  if (state.kind === "unavailable") {
    return <DailyUnavailable error={state.error} onRetry={retry} />;
  }

  return (
    <>
      <DiagnosticsCard
        id="daily"
        titleKey="diagnostics.daily.title"
        descriptionKey="diagnostics.daily.description"
      >
        <div
          data-slot="setting"
          data-diagnostics-daily-setting
          data-read-only={saving || undefined}
        >
          <div data-setting-copy>
            <div data-setting-heading>
              <span id="diagnostics-daily-enabled-label">{t("diagnostics.daily.enabled.label")}</span>
              <span
                data-diagnostics-daily-status
                role="status"
                aria-live="polite"
                aria-atomic="true"
              >
                <span data-slot="badge" data-status={saving ? "pending" : state.daily.enabled ? "ok" : "neutral"}>
                  {t(
                    saving
                      ? "diagnostics.daily.status.saving"
                      : state.daily.enabled
                        ? "diagnostics.daily.status.enabled"
                        : "diagnostics.daily.status.disabled"
                  )}
                </span>
              </span>
            </div>
            <p id="diagnostics-daily-enabled-description" data-setting-description>
              {t("diagnostics.daily.enabled.description", {
                time: state.daily.time,
                timezone: state.daily.timezone
              })}
            </p>
          </div>
          <div data-setting-control>
            <button
              id="diagnostics-daily-enabled"
              type="button"
              role="switch"
              data-slot="switch"
              data-diagnostics-daily-toggle
              aria-checked={state.daily.enabled}
              aria-disabled={saving ? "true" : undefined}
              aria-labelledby="diagnostics-daily-enabled-label"
              aria-describedby="diagnostics-daily-enabled-description"
              onClick={() => void toggleEnabled()}
            />
          </div>
        </div>
      </DiagnosticsCard>
      {feedback ? <DailyFeedbackNotice feedback={feedback} /> : null}
    </>
  );
}
