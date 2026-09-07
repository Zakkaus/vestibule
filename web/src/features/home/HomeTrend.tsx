import { LinkButton } from "@react-spectrum/s2/LinkButton";
import { Card, Content, Divider, Header, Heading, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { useConsoleSize } from "../../components/ConsoleProvider";
import { Icon } from "../../icons";
import type { StatsDay } from "../stats/api";
import type { HomeData } from "./useHomeData";

type ChartPoint = Readonly<{
  day: StatsDay;
  position: number;
  x: number;
  countY: number;
  countHeight: number;
  rateY: number;
}>;

type ChartModel = Readonly<{
  points: readonly ChartPoint[];
  width: number;
  countMax: number;
  countTicks: readonly number[];
  missingDays: number;
  expectedDays: number;
}>;

const chartGeometry = {
  left: 74,
  right: 24,
  top: 48,
  countBottom: 164,
  rateTop: 218,
  rateBottom: 324,
  dateLabelY: 364,
  widthPerDay: 96,
  height: 390
} as const;

function integerTicks(maximum: number): readonly number[] {
  if (maximum <= 0) {
    return [0];
  }
  const step = Math.max(1, Math.ceil(maximum / 4));
  const ticks: number[] = [];
  for (let value = 0; value <= maximum; value += step) {
    ticks.push(value);
  }
  if (ticks[ticks.length - 1] !== maximum) {
    ticks.push(maximum);
  }
  return ticks;
}

function expectedDates(from: string, to: string): readonly string[] {
  const start = new Date(`${from}T00:00:00Z`);
  const end = new Date(`${to}T00:00:00Z`);
  const dates: string[] = [];
  for (const current = new Date(start); current < end; current.setUTCDate(current.getUTCDate() + 1)) {
    dates.push(current.toISOString().slice(0, 10));
  }
  return dates;
}

function chartModel(data: HomeData): ChartModel {
  const trend = [...data.stats.trend].sort((left, right) => left.date.localeCompare(right.date));
  const expected = expectedDates(data.stats.range.from, data.stats.range.to);
  const returned = new Set(trend.map((day) => day.date));
  // One date domain prevents cached, out-of-window points from sharing an x slot.
  const dates = [...new Set([...expected, ...returned])].sort();
  const positions = new Map(dates.map((date, index) => [date, index]));
  const countMax = Math.max(1, ...trend.map((day) => day.challenges));
  const plotDays = Math.max(dates.length, 1);
  const plotWidth = plotDays * chartGeometry.widthPerDay;
  const points = trend.map((day) => {
    const position = positions.get(day.date)!;
    const x = chartGeometry.left + chartGeometry.widthPerDay * (position + 0.5);
    const countHeight = (day.challenges / countMax) * (chartGeometry.countBottom - chartGeometry.top);
    return {
      day,
      position,
      x,
      countY: chartGeometry.countBottom - countHeight,
      countHeight,
      rateY: chartGeometry.rateBottom - day.pass_rate * (chartGeometry.rateBottom - chartGeometry.rateTop)
    };
  });
  const missingDays = expected.filter((date) => !returned.has(date)).length;
  return {
    points,
    width: chartGeometry.left + plotWidth + chartGeometry.right,
    countMax,
    countTicks: integerTicks(countMax),
    missingDays,
    expectedDays: expected.length
  };
}

function ratePath(points: readonly ChartPoint[]): string {
  return points
    .map((point, index) => {
      const previous = points[index - 1];
      const connected = previous !== undefined && point.position === previous.position + 1;
      return `${connected ? "L" : "M"} ${point.x} ${point.rateY}`;
    })
    .join(" ");
}

function formatDay(date: string, formatter: Intl.DateTimeFormat) {
  return formatter.format(new Date(`${date}T00:00:00Z`));
}

function axisY(value: number, maximum: number, top: number, bottom: number) {
  return bottom - (value / maximum) * (bottom - top);
}

function CountPoints({
  chart,
  formatNumber,
  formatDate,
  formatRate,
  t
}: Readonly<{
  chart: ChartModel;
  formatNumber: Intl.NumberFormat;
  formatDate: Intl.DateTimeFormat;
  formatRate: Intl.NumberFormat;
  t: TFunction;
}>) {
  return (
    <>
      {chart.points.map((point) => {
        const date = formatDay(point.day.date, formatDate);
        const label = t("home.trend.daySummary", {
          date,
          count: formatNumber.format(point.day.challenges),
          passRate: formatRate.format(point.day.pass_rate)
        });
        return (
          <g
            key={`${point.day.date}-count`}
            className={style({ cursor: "pointer", outlineStyle: { default: "none", ":focus-visible": "solid" }, outlineColor: "focus-ring", outlineWidth: 2, outlineOffset: 4 })}
            data-home-chart-point
            data-home-chart-date={point.day.date}
            data-home-chart-series="count"
            data-home-chart-value={point.day.challenges}
            data-home-chart-pass-rate={point.day.pass_rate}
            tabIndex={0}
            role="img"
            aria-label={label}
            focusable="true"
          >
            <title data-home-chart-tooltip>{label}</title>
            {point.countHeight === 0 ? (
              <rect
                x={point.x - 22}
                y={chartGeometry.countBottom - 20}
                width="44"
                height="20"
                fill="transparent"
                opacity="0"
                aria-hidden="true"
              />
            ) : null}
            <rect
              x={point.x - 22}
              y={point.countY}
              width="44"
              height={point.countHeight}
              rx="4"
              className={style({ fill: "accent" })}
              data-home-chart-bar
            />
            <text
              x={point.x}
              y={point.countY - 8}
              textAnchor="middle"
              data-home-chart-count-label
            >
              {formatNumber.format(point.day.challenges)}
            </text>
          </g>
        );
      })}
    </>
  );
}

function CountPanel({
  chart,
  formatNumber,
  formatDate,
  formatRate,
  t
}: Readonly<{
  chart: ChartModel;
  formatNumber: Intl.NumberFormat;
  formatDate: Intl.DateTimeFormat;
  formatRate: Intl.NumberFormat;
  t: TFunction;
}>) {
  return (
    <g data-home-chart-panel="count">
      <text x={chartGeometry.left} y="16" data-home-chart-panel-label>
        {t("home.trend.countPanel")}
      </text>
      {chart.countTicks.map((tick) => {
        const y = axisY(tick, chart.countMax, chartGeometry.top, chartGeometry.countBottom);
        return (
          <g key={`count-tick-${tick}`} data-home-chart-tick={tick} data-home-chart-scale="count">
            <line
              x1={chartGeometry.left}
              y1={y}
              x2={chart.width - chartGeometry.right}
              y2={y}
              className={style({ stroke: "gray-300" })}
              strokeDasharray="3 5"
              strokeWidth={1}
              data-home-chart-grid
            />
            <text x={chartGeometry.left - 12} y={y + 4} textAnchor="end" data-home-chart-axis className={style({ color: "neutral-subdued" })}>
              {formatNumber.format(tick)}
            </text>
          </g>
        );
      })}
      <CountPoints
        chart={chart}
        formatNumber={formatNumber}
        formatDate={formatDate}
        formatRate={formatRate}
        t={t}
      />
    </g>
  );
}
function RatePanel({
  chart,
  formatNumber,
  formatDate,
  formatRate,
  t
}: Readonly<{
  chart: ChartModel;
  formatNumber: Intl.NumberFormat;
  formatDate: Intl.DateTimeFormat;
  formatRate: Intl.NumberFormat;
  t: TFunction;
}>) {
  const passPath = ratePath(chart.points);
  const rateTicks = [0, 0.25, 0.5, 0.75, 1];
  return (
    <g data-home-chart-panel="rate">
      <text x={chartGeometry.left} y="202" data-home-chart-panel-label>
        {t("home.trend.ratePanel")}
      </text>
      {rateTicks.map((tick) => {
        const y = axisY(tick, 1, chartGeometry.rateTop, chartGeometry.rateBottom);
        return (
          <g key={`rate-tick-${tick}`} data-home-chart-tick={tick} data-home-chart-scale="rate">
            <line
              x1={chartGeometry.left}
              y1={y}
              x2={chart.width - chartGeometry.right}
              y2={y}
              className={style({ stroke: "gray-300" })}
              strokeDasharray="3 5"
              strokeWidth={1}
              data-home-chart-grid
            />
            <text x={chartGeometry.left - 12} y={y + 4} textAnchor="end" data-home-chart-axis className={style({ color: "neutral-subdued" })}>
              {formatRate.format(tick)}
            </text>
          </g>
        );
      })}
      {chart.points.length > 1 ? <path d={passPath} data-home-chart-line fill="none" stroke="currentColor" strokeWidth={3} strokeLinecap="round" strokeLinejoin="round" className={style({ color: "accent" })} /> : null}
      {chart.points.map((point) => {
        const date = formatDay(point.day.date, formatDate);
        const label = t("home.trend.daySummary", {
          date,
          count: formatNumber.format(point.day.challenges),
          passRate: formatRate.format(point.day.pass_rate)
        });
        const rateLabelY = point.rateY <= chartGeometry.rateTop + 18
          ? point.rateY + 20
          : point.rateY - 12;
        return (
          <g
            key={`${point.day.date}-rate`}
            className={style({ cursor: "pointer", outlineStyle: { default: "none", ":focus-visible": "solid" }, outlineColor: "focus-ring", outlineWidth: 2, outlineOffset: 4 })}
            data-home-chart-point
            data-home-chart-date={point.day.date}
            data-home-chart-series="rate"
            data-home-chart-value={point.day.challenges}
            data-home-chart-pass-rate={point.day.pass_rate}
            tabIndex={0}
            role="img"
            aria-label={label}
            focusable="true"
          >
            <title data-home-chart-tooltip>{label}</title>
            <circle cx={point.x} cy={point.rateY} r="5" data-home-chart-dot fill="none" stroke="currentColor" strokeWidth={3} className={style({ color: "accent" })} />
            <text x={point.x} y={rateLabelY} textAnchor="middle" data-home-chart-rate-label>
              {formatRate.format(point.day.pass_rate)}
            </text>
            <text x={point.x} y={chartGeometry.dateLabelY} textAnchor="middle" data-home-chart-date-label className={style({ color: "neutral-subdued", pointerEvents: "none" })}>
              {formatDay(point.day.date, formatDate)}
            </text>
          </g>
        );
      })}
    </g>
  );
}

function TrendChart({ chart, locale }: Readonly<{ chart: ChartModel; locale: string }>) {
  const { t } = useTranslation();
  const formatNumber = useMemo(() => new Intl.NumberFormat(locale), [locale]);
  const formatRate = useMemo(
    () => new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 }),
    [locale]
  );
  const formatDate = useMemo(
    () => new Intl.DateTimeFormat(locale, { timeZone: "UTC", month: "short", day: "numeric" }),
    [locale]
  );

  return (
    <>
      <Content data-home-trend-scroll styles={style({ width: "full", minWidth: 0, overflowX: "auto", paddingY: 8, overscrollBehaviorX: "contain" })}>
        <svg
          data-home-trend-chart
          className={style({ display: "block", overflow: "visible", fill: "currentColor", color: "body", font: "ui-xs" })}
          width={chart.width}
          height={chartGeometry.height}
          viewBox={`0 0 ${chart.width} ${chartGeometry.height}`}
          role="group"
          aria-labelledby="home-trend-chart-title home-trend-chart-description"
        >
          <title id="home-trend-chart-title">{t("home.trend.chartTitle")}</title>
          <desc id="home-trend-chart-description">{t("home.trend.chartDescription")}</desc>
          <CountPanel
            chart={chart}
            formatNumber={formatNumber}
            formatDate={formatDate}
            formatRate={formatRate}
            t={t}
          />
          <RatePanel
            chart={chart}
            formatNumber={formatNumber}
            formatDate={formatDate}
            formatRate={formatRate}
            t={t}
          />
        </svg>
      </Content>
      <Content data-home-trend-legend aria-label={t("home.trend.legendLabel")} styles={style({ display: "flex", flexWrap: "wrap", gap: 16, font: "body-sm", color: "neutral-subdued" })}>
        <Text data-series="challenges">{t("home.trend.challenges")}</Text>
        <Text data-series="pass-rate">{t("home.trend.passRate")}</Text>
      </Content>
    </>
  );
}

export function HomeTrend({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { t, i18n } = useTranslation();
  const size = useConsoleSize("L");
  const chart = useMemo(
    () => (data.stats.trend.length > 0 ? chartModel(data) : null),
    [data]
  );
  const coverageText = chart
    ? t("home.trend.coverage", {
        shown: chart.expectedDays - chart.missingDays,
        expected: chart.expectedDays,
        missing: chart.missingDays
      })
    : null;

  return (
    <Card data-console-card data-home-section="trend" aria-labelledby="home-trend-title" styles={style({ width: "full", minWidth: 0 })}>
      <Header data-home-section-heading styles={style({ display: "flex", flexWrap: "wrap", alignItems: "end", justifyContent: "space-between", gap: 16 })}>
        <Content styles={style({ display: "grid", gap: 8 })}>
          <Heading level={2} id="home-trend-title" styles={style({ font: "heading", margin: 0 })}>{t("home.trend.title")}</Heading>
          <Text styles={style({ font: "body", color: "neutral-subdued" })}>{t("home.trend.description")}</Text>
          {coverageText ? <Text data-home-trend-coverage styles={style({ font: "body-sm", color: "neutral-subdued" })}>{coverageText}</Text> : null}
        </Content>
        <LinkButton
          href={`/stats${groupSearch}`}
          variant="secondary"
          fillStyle="outline"
          size={size}
          data-console-control
          data-control-size={size}
        >
          <Icon name="chartNoAxesCombined" />
          <Text>{t("home.trend.openStats")}</Text>
        </LinkButton>
      </Header>
      <Divider size="S" />
      {chart ? <TrendChart chart={chart} locale={i18n.language} /> : <Text data-home-trend-empty>{t("home.trend.empty")}</Text>}
    </Card>
  );
}
