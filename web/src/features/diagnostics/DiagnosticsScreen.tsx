import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { consoleApi, useConsoleSession } from "../../app/session";
import { StatusBadge } from "../../components/StatusBadge";
import type { ApiRequestError } from "../../lib/api";
import { loadDiagnostics, type Diagnostics } from "./api";

type DiagnosticsScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; diagnostics: Diagnostics }>
  | Readonly<{ kind: "access-denied" }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "diagnostics.errors.authenticationExpired",
  authentication_invalid: "diagnostics.errors.authenticationInvalid",
  diagnostics_unavailable: "diagnostics.errors.unavailable"
};

function diagnosticsErrorMessageKey(error: ApiRequestError): string {
  if (error.kind === "network") {
    return "diagnostics.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? "diagnostics.errors.loadUnavailable";
  }
  return "diagnostics.errors.invalidResponse";
}

function StateCard({
  id,
  titleKey,
  descriptionKey,
  role,
  live,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  role?: "alert";
  live?: "polite";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-diagnostics-state-card={id}
      role={role}
      aria-live={live}
      aria-labelledby={`diagnostics-${id}-title`}
    >
      <h2 id={`diagnostics-${id}-title`}>{t(titleKey)}</h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

function DiagnosticsCard({
  id,
  titleKey,
  descriptionKey,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-diagnostics-section={id} aria-labelledby={`diagnostics-${id}-title`}>
      <header data-diagnostics-section-heading>
        <div data-diagnostics-section-copy>
          <h2 id={`diagnostics-${id}-title`}>{t(titleKey)}</h2>
          <p>{t(descriptionKey)}</p>
        </div>
      </header>
      {children}
    </section>
  );
}

function DetailRow({ labelKey, name, children }: Readonly<{ labelKey: string; name: string; children: ReactNode }>) {
  const { t } = useTranslation();
  return (
    <div data-diagnostics-value={name}>
      <dt>{t(labelKey)}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function BooleanStatus({ value }: Readonly<{ value: boolean }>) {
  const { t } = useTranslation();
  return <StatusBadge tone={value ? "ok" : "error"}>{t(value ? "diagnostics.values.yes" : "diagnostics.values.no")}</StatusBadge>;
}

function ReadOnlyNotice() {
  const { t } = useTranslation();
  return (
    <aside data-slot="card" data-diagnostics-readonly aria-labelledby="diagnostics-readonly-title">
      <header data-diagnostics-readonly-heading>
        <h2 id="diagnostics-readonly-title">{t("diagnostics.readOnly.title")}</h2>
        <StatusBadge tone="neutral">{t("diagnostics.readOnly.badge")}</StatusBadge>
      </header>
      <p>{t("diagnostics.readOnly.description")}</p>
    </aside>
  );
}

type DiagnosticsFormatters = Readonly<{
  date: Intl.DateTimeFormat;
  number: Intl.NumberFormat;
}>;

function HealthSection({ health }: Readonly<{ health: Diagnostics["health"] }>) {
  return (
    <DiagnosticsCard
      id="health"
      titleKey="diagnostics.sections.health.title"
      descriptionKey="diagnostics.sections.health.description"
    >
      <dl data-diagnostics-values>
        <DetailRow name="live" labelKey="diagnostics.labels.live">
          <BooleanStatus value={health.live} />
        </DetailRow>
        <DetailRow name="ready" labelKey="diagnostics.labels.ready">
          <BooleanStatus value={health.ready} />
        </DetailRow>
        <DetailRow name="config-ready" labelKey="diagnostics.labels.configReady">
          <BooleanStatus value={health.configReady} />
        </DetailRow>
        <DetailRow name="telegram-ready" labelKey="diagnostics.labels.telegramReady">
          <BooleanStatus value={health.telegramReady} />
        </DetailRow>
      </dl>
    </DiagnosticsCard>
  );
}

function BotAPISection({
  botAPI,
  formatters
}: Readonly<{ botAPI: Diagnostics["botAPI"]; formatters: DiagnosticsFormatters }>) {
  const { t } = useTranslation();
  return (
    <DiagnosticsCard
      id="bot-api"
      titleKey="diagnostics.sections.botAPI.title"
      descriptionKey="diagnostics.sections.botAPI.description"
    >
      <dl data-diagnostics-values>
        <DetailRow name="last-heartbeat" labelKey="diagnostics.labels.lastHeartbeat">
          {botAPI.lastHeartbeatAt === null ? (
            <StatusBadge tone="neutral">{t("diagnostics.values.notMeasured")}</StatusBadge>
          ) : (
            <time dateTime={botAPI.lastHeartbeatAt}>
              {formatters.date.format(new Date(botAPI.lastHeartbeatAt))}
            </time>
          )}
        </DetailRow>
        <DetailRow name="latency" labelKey="diagnostics.labels.latency">
          {botAPI.latencyMilliseconds === null ? (
            <StatusBadge tone="neutral">{t("diagnostics.values.notMeasured")}</StatusBadge>
          ) : (
            <span data-diagnostics-metric="latency">
              {t("diagnostics.values.milliseconds", {
                value: formatters.number.format(botAPI.latencyMilliseconds)
              })}
            </span>
          )}
        </DetailRow>
      </dl>
    </DiagnosticsCard>
  );
}

function PersistenceSection({ persistence }: Readonly<{ persistence: Diagnostics["persistence"] }>) {
  const { t } = useTranslation();
  return (
    <DiagnosticsCard
      id="persistence"
      titleKey="diagnostics.sections.persistence.title"
      descriptionKey="diagnostics.sections.persistence.description"
    >
      <dl data-diagnostics-values>
        <DetailRow name="configured" labelKey="diagnostics.labels.configured">
          <BooleanStatus value={persistence.configured} />
        </DetailRow>
        <DetailRow name="durable" labelKey="diagnostics.labels.durable">
          <BooleanStatus value={persistence.durable} />
        </DetailRow>
        <DetailRow name="writable" labelKey="diagnostics.labels.writable">
          <BooleanStatus value={persistence.writable} />
        </DetailRow>
        <DetailRow name="last-error" labelKey="diagnostics.labels.lastError">
          {persistence.lastError === null ? (
            <StatusBadge tone="ok">{t("diagnostics.values.noFailure")}</StatusBadge>
          ) : (
            <span data-diagnostics-last-error>
              <StatusBadge tone="error">{t("diagnostics.values.failureRecorded")}</StatusBadge>
              <code>{persistence.lastError}</code>
            </span>
          )}
        </DetailRow>
      </dl>
    </DiagnosticsCard>
  );
}

function NotReportedSection() {
  const { t } = useTranslation();
  return (
    <DiagnosticsCard
      id="not-reported"
      titleKey="diagnostics.sections.notReported.title"
      descriptionKey="diagnostics.sections.notReported.description"
    >
      <ul data-diagnostics-unreported-list>
        <li data-diagnostics-unreported-item="permission-preflight">
          <strong>{t("diagnostics.labels.permissionPreflight")}</strong>
          <p>{t("diagnostics.notReported.permissionPreflight")}</p>
          <StatusBadge tone="neutral">{t("diagnostics.values.notReported")}</StatusBadge>
        </li>
        <li data-diagnostics-unreported-item="query-cache">
          <strong>{t("diagnostics.labels.queryCache")}</strong>
          <p>{t("diagnostics.notReported.queryCache")}</p>
          <StatusBadge tone="neutral">{t("diagnostics.values.notReported")}</StatusBadge>
        </li>
        <li data-diagnostics-unreported-item="query-rate-limit">
          <strong>{t("diagnostics.labels.queryRateLimit")}</strong>
          <p>{t("diagnostics.notReported.queryRateLimit")}</p>
          <StatusBadge tone="neutral">{t("diagnostics.values.notReported")}</StatusBadge>
        </li>
      </ul>
    </DiagnosticsCard>
  );
}

function DiagnosticsContent({
  diagnostics,
  formatters
}: Readonly<{ diagnostics: Diagnostics; formatters: DiagnosticsFormatters }>) {
  return (
    <div data-diagnostics-content>
      <ReadOnlyNotice />
      <HealthSection health={diagnostics.health} />
      <BotAPISection botAPI={diagnostics.botAPI} formatters={formatters} />
      <PersistenceSection persistence={diagnostics.persistence} />
      <NotReportedSection />
    </div>
  );
}

function useDiagnosticsScreenState(): Readonly<{
  screenState: DiagnosticsScreenState;
  reload: () => void;
}> {
  const session = useConsoleSession();
  const [screenState, setScreenState] = useState<DiagnosticsScreenState>({ kind: "loading" });
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => {
    let active = true;
    const stop = () => {
      active = false;
    };
    if (session.state === "loading" || session.state === "checking-groups") {
      setScreenState({ kind: "loading" });
      return stop;
    }
    if (session.state === "blocked") {
      setScreenState({ kind: "unavailable", error: session.error });
      return stop;
    }

    setScreenState({ kind: "loading" });
    void loadDiagnostics(consoleApi).then((result) => {
      if (!active) {
        return;
      }
      if (result.ok) {
        setScreenState({ kind: "loaded", diagnostics: result.data });
        return;
      }
      if (result.error.kind === "api" && result.error.code === "diagnostics_access_denied") {
        setScreenState({ kind: "access-denied" });
        return;
      }
      setScreenState({ kind: "unavailable", error: result.error });
    });

    return stop;
  }, [reloadVersion, session]);

  return {
    screenState,
    reload() {
      setReloadVersion((version) => version + 1);
    }
  };
}

export function DiagnosticsScreen() {
  const { i18n, t } = useTranslation();
  const { reload, screenState } = useDiagnosticsScreenState();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const formatters = useMemo<DiagnosticsFormatters>(
    () => ({
      date: new Intl.DateTimeFormat(locale, {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hourCycle: "h23"
      }),
      number: new Intl.NumberFormat(locale)
    }),
    [locale]
  );

  return (
    <section
      data-diagnostics-page
      data-diagnostics-state={screenState.kind}
      aria-busy={screenState.kind === "loading" || undefined}
      aria-labelledby="diagnostics-title"
    >
      <header data-page-heading>
        <h1 id="diagnostics-title">{t("diagnostics.title")}</h1>
        <p>{t("diagnostics.description")}</p>
      </header>
      {screenState.kind === "loading" ? (
        <StateCard
          id="loading"
          titleKey="diagnostics.loading.title"
          descriptionKey="diagnostics.loading.description"
          live="polite"
        />
      ) : null}
      {screenState.kind === "access-denied" ? (
        <StateCard
          id="access-denied"
          titleKey="diagnostics.accessDenied.title"
          descriptionKey="diagnostics.accessDenied.description"
          role="alert"
        />
      ) : null}
      {screenState.kind === "unavailable" ? (
        <StateCard
          id="unavailable"
          titleKey="diagnostics.unavailable.title"
          descriptionKey={diagnosticsErrorMessageKey(screenState.error)}
          role="alert"
        >
          <button
            type="button"
            data-slot="button"
            data-variant="outline"
            data-size="sm"
            onClick={reload}
          >
            {t("diagnostics.unavailable.retry")}
          </button>
        </StateCard>
      ) : null}
      {screenState.kind === "loaded" ? (
        <DiagnosticsContent diagnostics={screenState.diagnostics} formatters={formatters} />
      ) : null}
    </section>
  );
}
