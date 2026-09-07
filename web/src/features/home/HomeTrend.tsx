import {
  ColorSchemeContext, Content, Header, Heading, Link, Picker, PickerItem, Text
} from "@react-spectrum/s2";
import { size, style } from "@react-spectrum/s2/style" with { type: "macro" };
import { Chart, type ChartProps } from "@spectrum-charts/react-spectrum-charts-s2";
import { getS2ColorValue, getSpectrum2VegaConfig } from "@spectrum-charts/themes";
import { compile, type TopLevelSpec } from "vega-lite";
import type { Spec } from "vega";
import { useContext, useMemo, useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { useConsoleSize } from "../../components/ConsoleProvider";
import type { HomeData } from "./useHomeData";

type TrendPoint = {
  date: string;
  label: string;
  axisLabel: string;
  count: number;
  rate: number;
  summary: string;
};

type TrendModel = {
  points: TrendPoint[];
  missingDays: number;
  expectedDays: number;
  countSeries: string;
  rateSeries: string;
};

function expectedDates(from: string, to: string): readonly string[] {
  const start = new Date(`${from}T00:00:00Z`);
  const end = new Date(`${to}T00:00:00Z`);
  const dates: string[] = [];
  for (const current = new Date(start); current < end; current.setUTCDate(current.getUTCDate() + 1)) {
    dates.push(current.toISOString().slice(0, 10));
  }
  return dates;
}

function chartModel(data: HomeData, locale: string, t: TFunction): TrendModel {
  const expected = expectedDates(data.stats.range.from, data.stats.range.to);
  const returned = new Map(data.stats.trend.map((day) => [day.date, day]));
  const countSeries = t("home.trend.countSeries");
  const rateSeries = t("home.trend.rateSeries");
  const dateFormat = new Intl.DateTimeFormat(locale, { timeZone: "UTC", month: "short", day: "numeric" });
  const axisDateFormat = new Intl.DateTimeFormat(locale, { timeZone: "UTC", month: "numeric", day: "numeric" });
  const numberFormat = new Intl.NumberFormat(locale);
  const rateFormat = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 });
  const points = [...returned.values()].sort((left, right) => left.date.localeCompare(right.date)).map((day) => {
    const date = new Date(`${day.date}T00:00:00Z`);
    const label = dateFormat.format(date);
    const rateLabel = rateFormat.format(day.pass_rate);
    return {
      date: day.date,
      label,
      axisLabel: axisDateFormat.format(date),
      count: day.challenges,
      rate: day.pass_rate,
      summary: t("home.trend.daySummary", {
        date: label, count: numberFormat.format(day.challenges), passRate: rateLabel
      })
    };
  });
  return {
    points,
    missingDays: expected.filter((date) => !returned.has(date)).length,
    expectedDays: expected.length,
    countSeries,
    rateSeries
  };
}

function plotSpec(
  model: TrendModel,
  scheme: "light" | "dark",
  countAxisTitle: string,
  rateAxisTitle: string,
  legendTitle: string
): Spec {
  const theme = getSpectrum2VegaConfig(scheme);
  const labels = Object.fromEntries(model.points.map(({ date, axisLabel }) => [date, axisLabel]));
  const x = {
    field: "date",
    type: "ordinal" as const,
    scale: { domain: model.points.map(({ date }) => date) },
    axis: { title: null, labelAngle: 0, labelExpr: `${JSON.stringify(labels)}[datum.label]` }
  };
  const countColor = getS2ColorValue("blue-900", scheme);
  const rateColor = getS2ColorValue("seafoam-900", scheme);
  const seriesScale = {
    domain: [model.countSeries, model.rateSeries],
    range: [countColor, rateColor]
  };
  const chart: TopLevelSpec = {
    width: "container",
    autosize: { type: "fit", contains: "padding" },
    padding: theme.axis?.labelPadding as number,
    background: theme.background as string,
    config: { view: { stroke: theme.axis?.gridColor as string } },
    data: { values: model.points },
    layer: [
      {
        transform: [{ calculate: JSON.stringify(model.countSeries), as: "series" }],
        mark: { type: "bar" },
        encoding: {
          x,
          y: {
            field: "count",
            type: "quantitative",
            scale: { zero: true },
            axis: {
              orient: "left",
              title: countAxisTitle,
              grid: true,
              ticks: true,
              format: ",.0f",
              tickMinStep: 1
            }
          },
          color: {
            field: "series",
            type: "nominal",
            scale: seriesScale,
            legend: { title: legendTitle, orient: "bottom", symbolType: "circle" }
          },
          tooltip: { field: "summary", type: "nominal", title: "" }
        }
      },
      {
        mark: {
          type: "text",
          color: theme.text?.fill as string,
          baseline: "bottom",
          dy: -(theme.axis?.labelPadding as number)
        },
        encoding: {
          x,
          y: { field: "count", type: "quantitative", scale: { zero: true }, axis: null },
          text: { field: "count", type: "quantitative", format: ",.0f" }
        }
      },
      {
        transform: [{ calculate: JSON.stringify(model.rateSeries), as: "series" }],
        mark: {
          type: "line",
          invalid: "break-paths-show-domains",
          point: {
            filled: true,
            size: theme.symbol?.size as number,
            strokeWidth: theme.symbol?.strokeWidth as number
          }
        },
        encoding: {
          x,
          y: {
            field: "rate",
            type: "quantitative",
            scale: { domain: [0, 1] },
            axis: { orient: "right", title: rateAxisTitle, grid: false, ticks: true, format: ".0%" }
          },
          color: { field: "series", type: "nominal", scale: seriesScale },
          tooltip: { field: "summary", type: "nominal", title: "" }
        }
      },
      {
        mark: {
          type: "text",
          color: theme.text?.fill as string,
          baseline: "bottom",
          dy: -((theme.text?.fontSize as number) + (theme.axis?.labelPadding as number) * 2)
        },
        encoding: {
          x,
          y: { field: "count", type: "quantitative", scale: { zero: true }, axis: null },
          text: { field: "rate", type: "quantitative", format: ".1~%" }
        }
      }
    ],
    resolve: { scale: { y: "independent" } }
  };
  const spec = compile(chart).spec;
  // Labels must not intercept pointer input intended for the series below them.
  for (const mark of spec.marks ?? []) {
    if (mark.type === "text") mark.interactive = false;
  }
  // Spectrum Chart supplies measured width; do not run Vega-Lite's separate container observer.
  spec.signals = spec.signals?.filter((signal) => signal.name !== "width");
  spec.width = 0;
  spec.config = {
    ...spec.config,
    ...theme,
    legend: {
      ...theme.legend,
      layout: {
        ...theme.legend?.layout,
        bottom: { ...theme.legend?.layout?.bottom, anchor: "start", center: false }
      }
    }
  };
  return spec;
}

function TrendChart({ model, locale }: Readonly<{ model: TrendModel; locale: string }>) {
  const { t } = useTranslation();
  const controlSize = useConsoleSize("L");
  const preference = useContext(ColorSchemeContext);
  const systemScheme = useSyncExternalStore(subscribeColorScheme, systemColorScheme);
  const scheme = preference === "light" || preference === "dark" ? preference : systemScheme;
  const [selectedDate, setSelectedDate] = useState<string>();
  const selected = model.points.find((point) => point.date === selectedDate) ?? model.points[0];
  const spec = useMemo(() => plotSpec(
    model,
    scheme,
    t("home.trend.countAxis"),
    t("home.trend.rateAxis"),
    t("home.trend.legendLabel")
  ), [model, scheme, t]);
  const shared: ChartProps = {
    data: model.points,
    colorScheme: scheme,
    locale: locale === "en" ? "en-US" : { number: "zh-CN", time: locale === "zh-TW" ? "zh-TW" : "zh-CN" },
    animations: false,
    description: t("home.trend.chartDescription")
  };

  return (
    <Content data-home-trend-chart styles={style({ display: "grid", gap: `[${size(12)}]`, minWidth: 0 })}>
      <Content data-home-trend-scroll styles={style({ width: "full", minWidth: 0, overflowX: "auto", overscrollBehaviorX: "contain" })}>
        <Content styles={style({
          minWidth: { default: `[${size(400)}]`, lg: `[${size(360)}]`, isExtendedRange: `[${size(640)}]` },
          height: `[${size(200)}]`,
          display: "block"
        })({ isExtendedRange: model.points.length > 7 })}>
          <Chart {...shared} dataTestId="home-combined-chart" height="100%" padding={spec.padding as number} UNSAFE_vegaSpec={spec} />
        </Content>
      </Content>
      <Content styles={style({ display: "flex", flexWrap: "wrap", alignItems: "end", gap: `[${size(12)}]` })}>
        <Picker
          aria-label={t("home.trend.readDate")}
          size={controlSize}
          styles={style({ width: `[${size(128)}]` })}
          items={model.points}
          selectedKey={selected?.date}
          onSelectionChange={(key) => { if (key !== null) setSelectedDate(String(key)); }}
        >
          {(point) => <PickerItem id={point.date} textValue={point.label}>{point.label}</PickerItem>}
        </Picker>
        <Content aria-live="polite" aria-atomic="true" data-home-chart-reading styles={style({ flex: 1, minWidth: 0 })}>
          <Text>{selected?.summary}</Text>
        </Content>
      </Content>
    </Content>
  );
}

function subscribeColorScheme(listener: () => void) {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  media.addEventListener("change", listener);
  return () => media.removeEventListener("change", listener);
}

function systemColorScheme() {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function HomeTrend({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { t, i18n } = useTranslation();
  const model = useMemo(() => chartModel(data, i18n.language, t), [data, i18n.language, t]);
  const coverageText = t("home.trend.coverage", {
    shown: model.expectedDays - model.missingDays,
    expected: model.expectedDays,
    missing: model.missingDays
  });

  return (
    <Content data-home-section="trend" aria-labelledby="home-trend-title" styles={style({ display: "grid", gap: `[${size(8)}]`, width: "full", minWidth: 0 })}>
      <Header data-home-section-heading styles={style({ display: "flex", flexWrap: "wrap", alignItems: "end", justifyContent: "space-between", gap: `[${size(16)}]` })}>
        <Content styles={style({ display: "grid", gap: `[${size(8)}]` })}>
          <Heading level={2} id="home-trend-title" styles={style({ font: "heading", margin: 0 })}>{t("home.trend.title")}</Heading>
          {model.missingDays > 0 ? <Text data-home-trend-coverage styles={style({ font: "body-sm", color: "neutral-subdued" })}>{coverageText}</Text> : null}
        </Content>
        <Link href={`/stats${groupSearch}`} isStandalone>
          {t("home.trend.openStats")}
        </Link>
      </Header>
      {model.points.length > 0 ? <TrendChart model={model} locale={i18n.language} /> : <Text data-home-trend-empty>{t("home.trend.empty")}</Text>}
    </Content>
  );
}
