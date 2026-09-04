import { useCallback, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import {
  retryConsoleAccess,
  useConsoleSession
} from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import { Icon, type IconName } from "../../icons";
import type { StatsQuery } from "./api";
import {
  StatsResults,
  type StatsFormatters,
  useStatsFormatters
} from "./StatsViews";
import { type StatsDataState, useStatsData } from "./useStatsData";



type QueryError = "from" | "to" | "range" | "timezone";


const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "stats.errors.authenticationExpired",
  authentication_invalid: "stats.errors.authenticationInvalid",
  chat_access_denied: "stats.errors.accessDenied",
  chat_access_unavailable: "stats.errors.accessUnavailable",
  chat_not_found: "stats.errors.chatNotFound",
  invalid_stats_query: "stats.errors.invalidQuery",
  stats_unavailable: "stats.errors.unavailable"
};

function statsErrorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "stats.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? fallback;
  }
  return fallback;
}

function browserTimeZone(): string {
  try {
    const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    return timeZone.length > 0 ? timeZone : "UTC";
  } catch {
    return "UTC";
  }
}

function calendarDateInTimeZone(timeZone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit"
  }).formatToParts();
  const values: Record<string, string> = {};

  for (const part of parts) {
    if (part.type === "year" || part.type === "month" || part.type === "day") {
      values[part.type] = part.value;
    }
  }

  const year = values.year;
  const month = values.month;
  const day = values.day;
  return year && month && day ? `${year}-${month}-${day}` : new Date().toISOString().slice(0, 10);
}

function shiftCalendarDate(date: string, days: number): string {
  const shifted = new Date(`${date}T00:00:00Z`);
  shifted.setUTCDate(shifted.getUTCDate() + days);
  return shifted.toISOString().slice(0, 10);
}

function defaultStatsQuery(timeZone: string): StatsQuery {
  const today = calendarDateInTimeZone(timeZone);
  return {
    from: shiftCalendarDate(today, -6),
    to: shiftCalendarDate(today, 1),
    timezone: timeZone
  };
}

function validDateInput(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return false;
  }
  const parsed = new Date(`${value}T00:00:00Z`);
  return Number.isFinite(parsed.valueOf()) && parsed.toISOString().slice(0, 10) === value;
}

function statsQueryError(query: StatsQuery): QueryError | undefined {
  if (!validDateInput(query.from)) {
    return "from";
  }
  if (!validDateInput(query.to)) {
    return "to";
  }
  if (query.from > query.to) {
    return "range";
  }
  try {
    new Intl.DateTimeFormat("en", { timeZone: query.timezone }).format();
  } catch {
    return "timezone";
  }
  return undefined;
}



function StatsStateCard({
  id,
  icon,
  titleKey,
  descriptionKey,
  role,
  live,
  children
}: Readonly<{
  id: string;
  icon: IconName;
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
      data-stats-state-card={id}
      role={role}
      aria-live={live}
      aria-labelledby={`stats-${id}-title`}
    >
      <h2 id={`stats-${id}-title`} data-state-heading>
        <Icon name={icon} />
        {t(titleKey)}
      </h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

function StatsDateField({
  id,
  label,
  value,
  invalid,
  errorID,
  onChange
}: Readonly<{
  id: string;
  label: string;
  value: string;
  invalid: boolean;
  errorID: string | undefined;
  onChange: (value: string) => void;
}>) {
  return (
    <label data-stats-field>
      <span>{label}</span>
      <input
        id={id}
        type="date"
        data-slot="input"
        value={value}
        aria-invalid={invalid ? "true" : undefined}
        aria-describedby={invalid ? errorID : undefined}
        onChange={(event) => onChange(event.currentTarget.value)}
      />
    </label>
  );
}

function StatsTimeZoneField({
  value,
  invalid,
  errorID,
  onChange
}: Readonly<{
  value: string;
  invalid: boolean;
  errorID: string | undefined;
  onChange: (value: string) => void;
}>) {
  const { t } = useTranslation();

  return (
    <label data-stats-field>
      <span>{t("stats.filters.timezone")}</span>
      <input
        id="stats-timezone"
        type="text"
        data-slot="input"
        value={value}
        spellCheck={false}
        autoCapitalize="none"
        autoComplete="off"
        aria-invalid={invalid ? "true" : undefined}
        aria-describedby={`stats-timezone-help${invalid ? ` ${errorID}` : ""}`}
        onChange={(event) => onChange(event.currentTarget.value)}
      />
    </label>
  );
}

function StatsFilters({
  draft,
  browserZone,
  error,
  loading,
  onDraftChange,
  onSubmit
}: Readonly<{
  draft: StatsQuery;
  browserZone: string;
  error: QueryError | undefined;
  loading: boolean;
  onDraftChange: (field: keyof StatsQuery, value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
}>) {
  const { t } = useTranslation();
  const errorID = error ? "stats-query-error" : undefined;
  const fromInvalid = error === "from" || error === "range";
  const toInvalid = error === "to" || error === "range";

  return (
    <form data-slot="card" data-stats-filters onSubmit={onSubmit} noValidate>
      <fieldset>
        <legend>{t("stats.filters.title")}</legend>
        <div data-stats-filter-fields>
          <StatsDateField
            id="stats-from"
            label={t("stats.filters.from")}
            value={draft.from}
            invalid={fromInvalid}
            errorID={errorID}
            onChange={(value) => onDraftChange("from", value)}
          />
          <StatsDateField
            id="stats-to"
            label={t("stats.filters.to")}
            value={draft.to}
            invalid={toInvalid}
            errorID={errorID}
            onChange={(value) => onDraftChange("to", value)}
          />
          <StatsTimeZoneField
            value={draft.timezone}
            invalid={error === "timezone"}
            errorID={errorID}
            onChange={(value) => onDraftChange("timezone", value)}
          />
          <button
            type="submit"
            data-slot="button"
            data-variant="primary"
            aria-disabled={loading ? "true" : undefined}
          >
            <Icon name="slidersHorizontal" />
            {t("stats.filters.apply")}
          </button>
        </div>
        <p id="stats-timezone-help" data-stats-timezone-note>
          {t("stats.filters.timezoneDescription", { timezone: browserZone })}
        </p>
        {error ? (
          <p id={errorID} data-slot="field-error" role="alert">
            {t(`stats.filters.errors.${error}`)}
          </p>
        ) : null}
      </fieldset>
    </form>
  );
}


function StatsStateContent({
  state,
  onReload,
  formatters
}: Readonly<{
  state: StatsDataState;
  onReload: () => void;
  formatters: StatsFormatters;
}>) {
  const { t } = useTranslation();

  if (state.kind === "loaded") {
    return <StatsResults report={state.report} formatters={formatters} />;
  }
  if (state.kind === "loading") {
    return (
      <StatsStateCard
        id="loading"
        icon="loaderCircle"
        titleKey="stats.loading.title"
        descriptionKey="stats.loading.description"
        live="polite"
      />
    );
  }
  if (state.kind === "group-required") {
    return (
      <StatsStateCard
        id="group-required"
        icon="usersRound"
        titleKey="stats.groupRequired.title"
        descriptionKey="stats.groupRequired.description"
      >
        <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
          <Icon name="usersRound" />
          {t("stats.groupRequired.select")}
        </Link>
      </StatsStateCard>
    );
  }
  if (state.kind === "no-groups") {
    return (
      <StatsStateCard
        id="no-groups"
        icon="usersRound"
        titleKey="stats.noGroups.title"
        descriptionKey="stats.noGroups.description"
      />
    );
  }
  return (
    <StatsStateCard
      id="unavailable"
      icon="circleAlert"
      titleKey="stats.unavailable.title"
      descriptionKey={statsErrorMessageKey(state.error, "stats.errors.loadUnavailable")}
      role="alert"
    >
      <button type="button" data-slot="button" data-variant="outline" data-size="sm" onClick={onReload}>
        <Icon name="refreshCw" />
        {t("stats.unavailable.retry")}
      </button>
    </StatsStateCard>
  );
}

function useStatsScreenState(browserZone: string) {
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const selectedGroupID = searchParams.get("group");
  const chatID = selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID) ? selectedGroupID : undefined;
  const initialQuery = useMemo(() => defaultStatsQuery(browserZone), [browserZone]);
  const [draft, setDraft] = useState<StatsQuery>(initialQuery);
  const [requestedQuery, setRequestedQuery] = useState<StatsQuery>(initialQuery);
  const [queryError, setQueryError] = useState<QueryError>();
  const [reloadVersion, setReloadVersion] = useState(0);
  const [submitting, setSubmitting] = useState(false);
  const finishRequest = useCallback(() => {
    setSubmitting(false);
  }, []);
  const screenState = useStatsData(session, chatID, requestedQuery, reloadVersion, finishRequest);
  const canQuery = session.state === "ready" && chatID !== undefined;
  const loading = (screenState.kind === "loading" || submitting) && canQuery;

  function updateDraft(field: keyof StatsQuery, value: string): void {
    setDraft((current) => ({ ...current, [field]: value }));
    setQueryError(undefined);
  }

  function submitQuery(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    if (loading) {
      return;
    }

    const nextQuery = { ...draft, timezone: draft.timezone.trim() };
    const nextError = statsQueryError(nextQuery);
    setQueryError(nextError);
    if (nextError) {
      return;
    }
    setSubmitting(true);
    setDraft(nextQuery);
    if (
      requestedQuery.from === nextQuery.from &&
      requestedQuery.to === nextQuery.to &&
      requestedQuery.timezone === nextQuery.timezone
    ) {
      setReloadVersion((version) => version + 1);
      return;
    }
    setRequestedQuery(nextQuery);
  }

  function reload(): void {
    if (loading || retryConsoleAccess(session)) {
      return;
    }
    setSubmitting(true);
    setReloadVersion((version) => version + 1);
  }

  return { canQuery, draft, loading, queryError, reload, screenState, submitting, submitQuery, updateDraft };
}

export function StatsScreen() {
  const { t, i18n } = useTranslation();
  const browserZone = useMemo(browserTimeZone, []);
  const stats = useStatsScreenState(browserZone);
  const formatters = useStatsFormatters(i18n.language);

  return (
    <section
      data-stats-page
      data-stats-state={stats.screenState.kind}
      aria-busy={stats.screenState.kind === "loading" || stats.submitting ? true : undefined}
      aria-labelledby="stats-title"
    >
      <header data-page-heading>
        <h1 id="stats-title">
          <Icon name="chartNoAxesCombined" />
          {t("stats.title")}
        </h1>
        <p>{t("stats.description")}</p>
      </header>
      {stats.canQuery ? (
        <StatsFilters
          draft={stats.draft}
          browserZone={browserZone}
          error={stats.queryError}
          loading={stats.loading}
          onDraftChange={stats.updateDraft}
          onSubmit={stats.submitQuery}
        />
      ) : null}
      <StatsStateContent state={stats.screenState} onReload={stats.reload} formatters={formatters} />
    </section>
  );
}
