import { useMemo } from "react";
import { useTranslation } from "react-i18next";

import type {
  StatsDay,
  StatsInterception,
  StatsOutcome,
  StatsReport
} from "./api";

export type StatsFormatters = Readonly<{
  number: Intl.NumberFormat;
  percent: Intl.NumberFormat;
  date: Intl.DateTimeFormat;
  shortDate: Intl.DateTimeFormat;
}>;

type TrendChartPoint = Readonly<{
  day: StatsDay;
  x: number;
  barX: number;
  barWidth: number;
  barY: number;
  barHeight: number;
  passRateY: number;
  showLabel: boolean;
}>;

export function useStatsFormatters(locale: string): StatsFormatters {
  return useMemo(
    () => ({
      number: new Intl.NumberFormat(locale),
      percent: new Intl.NumberFormat(locale, {
        style: "percent",
        maximumFractionDigits: 1
      }),
      date: new Intl.DateTimeFormat(locale, {
        timeZone: "UTC",
        year: "numeric",
        month: "short",
        day: "numeric"
      }),
      shortDate: new Intl.DateTimeFormat(locale, {
        timeZone: "UTC",
        month: "2-digit",
        day: "2-digit"
      })
    }),
    [locale]
  );
}

function formatDate(value: string, formatter: Intl.DateTimeFormat): string {
  return formatter.format(new Date(`${value}T00:00:00Z`));
}

function Summary({
  outcome,
  formatters
}: Readonly<{
  outcome: StatsOutcome;
  formatters: StatsFormatters;
}>) {
  const { t } = useTranslation();
  const entries = [
    ["challenges", formatters.number.format(outcome.challenges)],
    ["approved", formatters.number.format(outcome.approved)],
    ["declined", formatters.number.format(outcome.declined)],
    ["banned", formatters.number.format(outcome.banned)],
    ["expired", formatters.number.format(outcome.expired)],
    ["passRate", formatters.percent.format(outcome.pass_rate)]
  ] as const;

  return (
    <section data-stats-summary aria-labelledby="stats-summary-title">
      <header data-stats-section-heading>
        <h2 id="stats-summary-title">{t("stats.summary.title")}</h2>
      </header>
      <div data-stats-kpis>
        {entries.map(([label, value]) => (
          <dl key={label} data-slot="card" data-stats-kpi>
            <dt>{t(`stats.summary.${label}`)}</dt>
            <dd>{value}</dd>
          </dl>
        ))}
      </div>
    </section>
  );
}

function trendChartPoints(trend: readonly StatsDay[]): Readonly<{
  maxChallenges: number;
  points: readonly TrendChartPoint[];
  passRatePath: string;
}> {
  const plotLeft = 48;
  const plotRight = 600;
  const plotTop = 24;
  const plotBottom = 196;
  const plotWidth = plotRight - plotLeft;
  const plotHeight = plotBottom - plotTop;
  const maxChallenges = Math.max(1, ...trend.map((day) => day.challenges));
  const step = plotWidth / Math.max(1, trend.length);
  const barWidth = Math.max(1, Math.min(24, step * 0.58));
  const labelEvery = Math.max(1, Math.ceil(trend.length / 7));
  const points = trend.map((day, index) => {
    const x = plotLeft + step * index + step / 2;
    const barHeight = (day.challenges / maxChallenges) * plotHeight;
    return {
      day,
      x,
      barX: x - barWidth / 2,
      barWidth,
      barY: plotBottom - barHeight,
      barHeight,
      passRateY: plotBottom - day.pass_rate * plotHeight,
      showLabel: index % labelEvery === 0 || index === trend.length - 1
    };
  });
  const passRatePath = points
    .map((point, index) => `${index === 0 ? "M" : "L"} ${point.x} ${point.passRateY}`)
    .join(" ");

  return { maxChallenges, points, passRatePath };
}

function TrendChart({
  trend,
  formatters
}: Readonly<{
  trend: readonly StatsDay[];
  formatters: StatsFormatters;
}>) {
  const { t } = useTranslation();
  const chart = trendChartPoints(trend);
  const plotLeft = 48;
  const plotRight = 600;
  const plotTop = 24;
  const plotBottom = 196;
  const middle = (plotTop + plotBottom) / 2;

  return (
    <div data-stats-chart-scroll>
      <svg
        data-stats-chart-svg
        viewBox="0 0 640 240"
        role="img"
        aria-labelledby="stats-trend-chart-title"
      >
        <title id="stats-trend-chart-title">{t("stats.trend.chartDescription")}</title>
        <line x1={plotLeft} y1={plotBottom} x2={plotRight} y2={plotBottom} stroke="var(--border)" />
        <line x1={plotLeft} y1={middle} x2={plotRight} y2={middle} stroke="var(--border)" />
        <line x1={plotLeft} y1={plotTop} x2={plotRight} y2={plotTop} stroke="var(--border)" />
        <text x={40} y={plotBottom + 4} textAnchor="end" data-stats-chart-label>
          {formatters.number.format(0)}
        </text>
        <text x={40} y={plotTop + 4} textAnchor="end" data-stats-chart-label>
          {formatters.number.format(chart.maxChallenges)}
        </text>
        <text x={608} y={plotBottom + 4} data-stats-chart-label>
          {formatters.percent.format(0)}
        </text>
        <text x={608} y={plotTop + 4} data-stats-chart-label>
          {formatters.percent.format(1)}
        </text>
        {chart.points.map((point) => (
          <rect
            key={point.day.date}
            x={point.barX}
            y={point.barY}
            width={point.barHeight === 0 ? 0 : point.barWidth}
            height={point.barHeight}
            fill="var(--fill-info)"
          />
        ))}
        <path
          d={chart.passRatePath}
          fill="none"
          stroke="var(--fill-ok)"
          strokeWidth="2.5"
          strokeLinejoin="round"
          strokeLinecap="round"
        />
        {chart.points.map((point) => (
          <g key={`${point.day.date}-point`}>
            <circle
              cx={point.x}
              cy={point.passRateY}
              r="3"
              fill="var(--card)"
              stroke="var(--fill-ok)"
              strokeWidth="2"
            />
            {point.showLabel ? (
              <text x={point.x} y="218" textAnchor="middle" data-stats-chart-label>
                {formatDate(point.day.date, formatters.shortDate)}
              </text>
            ) : null}
          </g>
        ))}
      </svg>
    </div>
  );
}

function TrendSection({
  trend,
  formatters
}: Readonly<{
  trend: readonly StatsDay[];
  formatters: StatsFormatters;
}>) {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-stats-chart-section aria-labelledby="stats-trend-title">
      <header data-stats-section-heading>
        <h2 id="stats-trend-title">{t("stats.trend.title")}</h2>
        <p>{t("stats.trend.description")}</p>
      </header>
      {trend.length === 0 ? <p data-stats-empty>{t("stats.trend.noDays")}</p> : <TrendChart trend={trend} formatters={formatters} />}
      {trend.length > 0 ? (
        <div data-stats-chart-legend aria-label={t("stats.trend.legendLabel")}>
          <span data-stats-chart-series="challenges">{t("stats.trend.challenges")}</span>
          <span data-stats-chart-series="pass-rate">{t("stats.trend.passRate")}</span>
        </div>
      ) : null}
    </section>
  );
}

function DailyResults({
  trend,
  formatters
}: Readonly<{
  trend: readonly StatsDay[];
  formatters: StatsFormatters;
}>) {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-stats-table-section aria-labelledby="stats-daily-title">
      <header data-stats-section-heading>
        <h2 id="stats-daily-title">{t("stats.daily.title")}</h2>
        <p>{t("stats.daily.description")}</p>
      </header>
      {trend.length === 0 ? (
        <p data-stats-empty>{t("stats.trend.noDays")}</p>
      ) : (
        <div data-stats-table-scroll>
          <table data-slot="table" data-stats-trend-table>
            <thead>
              <tr>
                <th scope="col" data-slot="table-head">{t("stats.daily.date")}</th>
                <th scope="col" data-slot="table-head" data-stats-number>{t("stats.daily.challenges")}</th>
                <th scope="col" data-slot="table-head" data-stats-number>{t("stats.daily.approved")}</th>
                <th scope="col" data-slot="table-head" data-stats-number>{t("stats.daily.declined")}</th>
                <th scope="col" data-slot="table-head" data-stats-number>{t("stats.daily.banned")}</th>
                <th scope="col" data-slot="table-head" data-stats-number>{t("stats.daily.expired")}</th>
                <th scope="col" data-slot="table-head" data-stats-number>{t("stats.daily.passRate")}</th>
              </tr>
            </thead>
            <tbody>
              {trend.map((day) => (
                <tr key={day.date} data-slot="table-row">
                  <td data-slot="table-cell"><time dateTime={day.date}>{formatDate(day.date, formatters.date)}</time></td>
                  <td data-slot="table-cell" data-stats-number>{formatters.number.format(day.challenges)}</td>
                  <td data-slot="table-cell" data-stats-number>{formatters.number.format(day.approved)}</td>
                  <td data-slot="table-cell" data-stats-number>{formatters.number.format(day.declined)}</td>
                  <td data-slot="table-cell" data-stats-number>{formatters.number.format(day.banned)}</td>
                  <td data-slot="table-cell" data-stats-number>{formatters.number.format(day.expired)}</td>
                  <td data-slot="table-cell" data-stats-number>{formatters.percent.format(day.pass_rate)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function Interceptions({
  interceptions,
  formatters
}: Readonly<{
  interceptions: readonly StatsInterception[];
  formatters: StatsFormatters;
}>) {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-stats-table-section aria-labelledby="stats-interceptions-title">
      <header data-stats-section-heading>
        <h2 id="stats-interceptions-title">{t("stats.interceptions.title")}</h2>
        <p>{t("stats.interceptions.description")}</p>
      </header>
      {interceptions.length === 0 ? (
        <p data-stats-empty>{t("stats.interceptions.empty")}</p>
      ) : (
        <div data-stats-table-scroll>
          <table data-slot="table" data-stats-interceptions-table>
            <thead>
              <tr>
                <th scope="col" data-slot="table-head">{t("stats.interceptions.kind")}</th>
                <th scope="col" data-slot="table-head" data-stats-number>{t("stats.interceptions.count")}</th>
              </tr>
            </thead>
            <tbody>
              {interceptions.map((interception, index) => (
                <tr key={`${interception.kind}-${index}`} data-slot="table-row">
                  <td data-slot="table-cell"><code data-stats-kind>{interception.kind}</code></td>
                  <td data-slot="table-cell" data-stats-number>{formatters.number.format(interception.count)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

export function StatsResults({
  report,
  formatters
}: Readonly<{
  report: StatsReport;
  formatters: StatsFormatters;
}>) {
  const { t } = useTranslation();
  const hasResults = report.summary.challenges > 0;

  return (
    <div data-stats-results>
      <header data-stats-results-heading>
        <p>
          {t("stats.range.applied", {
            from: formatDate(report.range.from, formatters.date),
            to: formatDate(report.range.to, formatters.date),
            timezone: report.range.timezone
          })}
        </p>
      </header>
      {!hasResults ? <p className="surface-raised" data-stats-zero-state>{t("stats.zero")}</p> : null}
      <Summary outcome={report.summary} formatters={formatters} />
      <TrendSection trend={report.trend} formatters={formatters} />
      <DailyResults trend={report.trend} formatters={formatters} />
      <Interceptions interceptions={report.interceptions} formatters={formatters} />
    </div>
  );
}
