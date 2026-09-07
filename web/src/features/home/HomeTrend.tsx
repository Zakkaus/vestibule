import { LinkButton } from "@react-spectrum/s2/LinkButton";
import {
  Card, ColorSchemeContext, Content, Divider, Header, Heading, Picker, PickerItem, Text
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
import { Icon } from "../../icons";
import type { HomeData } from "./useHomeData";

type TrendPoint = {
  date: string;
  label: string;
  count: number | null;
  rate: number | null;
  hasReading: boolean;
  countSeries: string;
  rateSeries: string;
  summary: string;
};

type TrendModel = {
  points: TrendPoint[];
  readings: TrendPoint[];
  missingDays: number;
  expectedDays: number;
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
  const dates = [...new Set([...expected, ...returned.keys()])].sort();
  const dateFormat = new Intl.DateTimeFormat(locale, { timeZone: "UTC", month: "short", day: "numeric" });
  const numberFormat = new Intl.NumberFormat(locale);
  const rateFormat = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 });
  const points = dates.map((date) => {
    const day = returned.get(date);
    const label = dateFormat.format(new Date(`${date}T00:00:00Z`));
    const rateLabel = day ? rateFormat.format(day.pass_rate) : undefined;
    return {
      date, label,
      count: day?.challenges ?? null,
      rate: day?.pass_rate ?? null,
      hasReading: day !== undefined,
      countSeries: t("home.trend.challenges"),
      rateSeries: t("home.trend.passRate"),
      summary: day ? t("home.trend.daySummary", {
        date: label, count: numberFormat.format(day.challenges), passRate: rateLabel
      }) : t("home.trend.missingDay", { date: label })
    };
  });
  return {
    points,
    readings: points.filter((point) => point.hasReading),
    missingDays: expected.filter((date) => !returned.has(date)).length,
    expectedDays: expected.length
  };
}

function subscribeColorScheme(listener: () => void) {
  const media = window.matchMedia("(prefers-color-scheme: dark)");
  media.addEventListener("change", listener);
  return () => media.removeEventListener("change", listener);
}

function systemColorScheme() {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function plotSpec(model: TrendModel, metric: "count" | "rate", scheme: "light" | "dark", title: string): Spec {
  const rate = metric === "rate";
  const theme = getSpectrum2VegaConfig(scheme);
  const labels = Object.fromEntries(model.points.map(({ date, label }) => [date, label]));
  const chart: TopLevelSpec = {
    width: "container",
    background: theme.background as string,
    config: { view: { stroke: theme.axis?.gridColor as string } },
    data: { values: model.points },
    encoding: {
      x: {
        field: "date", type: "ordinal", scale: { domain: model.points.map(({ date }) => date) },
        axis: { title: null, labelAngle: 0, labelExpr: `${JSON.stringify(labels)}[datum.label]` }
      },
      y: {
        field: metric, type: "quantitative",
        scale: rate ? { domain: [0, 1] } : { zero: true },
        axis: { title, grid: true, ticks: true, format: rate ? ".0%" : ",.0f", ...(rate ? {} : { tickMinStep: 1 }) }
      }
    },
    layer: [
      {
        mark: rate ? {
          type: "line", invalid: "break-paths-show-domains",
          point: { filled: true, size: theme.symbol?.size as number, strokeWidth: theme.symbol?.strokeWidth as number }
        } : { type: "bar" },
        encoding: {
          description: { field: "summary" },
          color: {
            field: rate ? "rateSeries" : "countSeries", type: "nominal",
            scale: { range: [getS2ColorValue(rate ? "seafoam-900" : "blue-900", scheme)] },
            legend: { title: null, orient: "bottom", symbolType: "circle" }
          },
          tooltip: { field: "summary", type: "nominal", title: "" }
        }
      },
      {
        transform: [{ filter: { field: "hasReading", equal: true } }],
        mark: { type: "text", color: theme.text?.fill as string, baseline: "bottom", dy: -(theme.axis?.labelPadding as number) },
        encoding: { text: { field: metric, type: "quantitative", format: rate ? ".1~%" : ",.0f" } }
      }
    ]
  };
  const spec = compile(chart).spec;
  // Spectrum Chart supplies measured width; do not run Vega-Lite's separate container observer.
  spec.signals = spec.signals?.filter((signal) => signal.name !== "width");
  spec.width = 0;
  spec.config = {
    ...spec.config, ...theme,
    legend: { ...theme.legend, layout: { ...theme.legend?.layout, bottom: { ...theme.legend?.layout?.bottom, anchor: "start", center: false } } }
  };
  return spec;
}

function TrendChart({ model, locale }: Readonly<{ model: TrendModel; locale: string }>) {
  const { t } = useTranslation();
  const preference = useContext(ColorSchemeContext);
  const systemScheme = useSyncExternalStore(subscribeColorScheme, systemColorScheme);
  const scheme = preference === "light" || preference === "dark" ? preference : systemScheme;
  const [selectedDate, setSelectedDate] = useState<string>();
  const selected = model.readings.find((point) => point.date === selectedDate) ?? model.readings[0];
  const specs = useMemo(() => ({
    count: plotSpec(model, "count", scheme, t("home.trend.countPanel")),
    rate: plotSpec(model, "rate", scheme, t("home.trend.ratePanel"))
  }), [model, scheme, t]);
  const shared: ChartProps = {
    data: model.points,
    colorScheme: scheme,
    locale: locale === "en" ? "en-US" : { number: "zh-CN", time: locale === "zh-TW" ? "zh-TW" : "zh-CN" },
    animations: false,
    description: t("home.trend.chartDescription")
  };

  return (
    <Content data-home-trend-chart styles={style({ display: "grid", gap: `[${size(16)}]`, minWidth: 0 })}>
      <Content data-home-trend-scroll styles={style({ width: "full", minWidth: 0, overflowX: "auto", overscrollBehaviorX: "contain" })}>
        <Content styles={style({
          minWidth: { default: `[${size(640)}]`, isExtendedRange: `[${size(800)}]` },
          display: "grid", gap: `[${size(24)}]`
        })({ isExtendedRange: model.points.length > 7 })}>
          <Chart {...shared} dataTestId="home-count-chart" UNSAFE_vegaSpec={specs.count} />
          <Chart {...shared} dataTestId="home-rate-chart" UNSAFE_vegaSpec={specs.rate} />
        </Content>
      </Content>
      <Picker
        label={t("home.trend.readDate")}
        items={model.readings}
        selectedKey={selected?.date}
        onSelectionChange={(key) => { if (key !== null) setSelectedDate(String(key)); }}
      >
        {(point) => <PickerItem id={point.date} textValue={point.label}>{point.label}</PickerItem>}
      </Picker>
      <Content aria-live="polite" aria-atomic="true" data-home-chart-reading>
        <Text>{selected?.summary}</Text>
      </Content>
    </Content>
  );
}

export function HomeTrend({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { t, i18n } = useTranslation();
  const controlSize = useConsoleSize("L");
  const model = useMemo(() => chartModel(data, i18n.language, t), [data, i18n.language, t]);
  const coverageText = t("home.trend.coverage", {
    shown: model.expectedDays - model.missingDays,
    expected: model.expectedDays,
    missing: model.missingDays
  });

  return (
    <Card data-console-card data-home-section="trend" aria-labelledby="home-trend-title" styles={style({ width: "full", minWidth: 0 })}>
      <Header data-home-section-heading styles={style({ display: "flex", flexWrap: "wrap", alignItems: "end", justifyContent: "space-between", gap: `[${size(16)}]` })}>
        <Content styles={style({ display: "grid", gap: `[${size(8)}]` })}>
          <Heading level={2} id="home-trend-title" styles={style({ font: "heading", margin: 0 })}>{t("home.trend.title")}</Heading>
          <Text styles={style({ font: "body", color: "neutral-subdued" })}>{t("home.trend.description")}</Text>
          {model.readings.length > 0 ? <Text data-home-trend-coverage styles={style({ font: "body-sm", color: "neutral-subdued" })}>{coverageText}</Text> : null}
        </Content>
        <LinkButton
          href={`/stats${groupSearch}`}
          variant="secondary"
          fillStyle="outline"
          size={controlSize}
          data-console-control
          data-control-size={controlSize}
        >
          <Icon name="chartNoAxesCombined" />
          <Text>{t("home.trend.openStats")}</Text>
        </LinkButton>
      </Header>
      <Divider size="S" />
      {model.readings.length > 0 ? <TrendChart model={model} locale={i18n.language} /> : <Text data-home-trend-empty>{t("home.trend.empty")}</Text>}
    </Card>
  );
}
