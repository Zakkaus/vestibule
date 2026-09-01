import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export type StatsOutcome = Readonly<{
  challenges: number;
  approved: number;
  declined: number;
  banned: number;
  expired: number;
  pass_rate: number;
}>;

export type StatsRange = Readonly<{
  from: string;
  to: string;
  timezone: string;
}>;

export type StatsDay = Readonly<{
  date: string;
} & StatsOutcome>;

export type StatsInterception = Readonly<{
  kind: string;
  count: number;
}>;

export type StatsReport = Readonly<{
  range: StatsRange;
  summary: StatsOutcome;
  trend: readonly StatsDay[];
  interceptions: readonly StatsInterception[];
}>;

export type StatsQuery = Readonly<{
  from: string;
  to: string;
  timezone: string;
}>;

function nonNegativeIntegerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
    ? value
    : undefined;
}

function dateFromPayload(value: unknown): string | undefined {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    return undefined;
  }

  const parsed = new Date(`${value}T00:00:00Z`);
  return Number.isFinite(parsed.valueOf()) && parsed.toISOString().slice(0, 10) === value
    ? value
    : undefined;
}

function outcomeFromPayload(value: unknown): StatsOutcome | undefined {
  const outcome = objectFromPayload(value);
  if (!outcome) {
    return undefined;
  }

  const challenges = nonNegativeIntegerFromPayload(outcome.challenges);
  const approved = nonNegativeIntegerFromPayload(outcome.approved);
  const declined = nonNegativeIntegerFromPayload(outcome.declined);
  const banned = nonNegativeIntegerFromPayload(outcome.banned);
  const expired = nonNegativeIntegerFromPayload(outcome.expired);
  const passRate = outcome.pass_rate;

  if (
    challenges === undefined ||
    approved === undefined ||
    declined === undefined ||
    banned === undefined ||
    expired === undefined ||
    typeof passRate !== "number" ||
    !Number.isFinite(passRate) ||
    passRate < 0 ||
    passRate > 1
  ) {
    return undefined;
  }

  return {
    challenges,
    approved,
    declined,
    banned,
    expired,
    pass_rate: passRate
  };
}

function rangeFromPayload(value: unknown): StatsRange | undefined {
  const range = objectFromPayload(value);
  if (!range) {
    return undefined;
  }

  const from = dateFromPayload(range.from);
  const to = dateFromPayload(range.to);
  const timezone = typeof range.timezone === "string" && range.timezone.length > 0
    ? range.timezone
    : undefined;

  return from && to && timezone ? { from, to, timezone } : undefined;
}

function trendFromPayload(value: unknown): readonly StatsDay[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const trend: StatsDay[] = [];
  for (const candidate of value) {
    const day = objectFromPayload(candidate);
    const date = day ? dateFromPayload(day.date) : undefined;
    const outcome = outcomeFromPayload(candidate);
    if (!date || !outcome) {
      return undefined;
    }
    trend.push({ date, ...outcome });
  }

  return trend;
}

function interceptionsFromPayload(value: unknown): readonly StatsInterception[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const interceptions: StatsInterception[] = [];
  for (const candidate of value) {
    const interception = objectFromPayload(candidate);
    const kind = interception?.kind;
    const count = interception ? nonNegativeIntegerFromPayload(interception.count) : undefined;
    if (typeof kind !== "string" || count === undefined) {
      return undefined;
    }
    interceptions.push({ kind, count });
  }

  return interceptions;
}

export function statsReportFromPayload(payload: unknown): StatsReport | undefined {
  const report = objectFromPayload(payload);
  if (!report) {
    return undefined;
  }

  const range = rangeFromPayload(report.range);
  const summary = outcomeFromPayload(report.summary);
  const trend = trendFromPayload(report.trend);
  const interceptions = interceptionsFromPayload(report.interceptions);

  return range && summary && trend && interceptions
    ? { range, summary, trend, interceptions }
    : undefined;
}

export function loadStats(
  transport: ApiTransport,
  chatID: string,
  query: StatsQuery
): Promise<ApiResult<StatsReport>> {
  const search = new URLSearchParams({
    from: query.from,
    to: query.to,
    timezone: query.timezone
  });
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/stats?${search.toString()}`, {
    parse: statsReportFromPayload
  });
}
