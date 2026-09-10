import {
  ColorSchemeContext,
  Content,
  Header,
  Heading,
  Link,
  Picker,
  PickerItem,
  Text
} from "@react-spectrum/s2";
import { size, style } from "@react-spectrum/s2/style" with { type: "macro" };
import { sectionSurface } from "./surface";
import { Axis, Bar, BarDirectLabel, Chart, ChartInspect, Line, type ChartProps } from "@spectrum-charts/react-spectrum-charts-s2";
import { useContext, useMemo, useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { useConsoleSize } from "../../components/ConsoleProvider";
import { Icon } from "../../icons";
import type { HomeData } from "./useHomeData";

type TrendPoint = {
  date: string;
  label: string;
  axisLabel: string;
  count: number;
  rate: number;
  // LinePointAnnotation prints a field, it does not format one.
  rateLabel: string;
  // Line only emits annotation marks for rows flagged as static points.
  alwaysPoint: true;
  summary: string;
};

type TrendModel = {
  points: TrendPoint[];
  missingDays: number;
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
      rateLabel,
      alwaysPoint: true as const,
      summary: t("home.trend.daySummary", {
        date: label, count: numberFormat.format(day.challenges), passRate: rateLabel
      })
    };
  });
  return {
    points,
    missingDays: expected.filter((date) => !returned.has(date)).length,
    countSeries,
    rateSeries
  };
}

function TrendChart({ model, locale }: Readonly<{ model: TrendModel; locale: string }>) {
  const { t } = useTranslation();
  const controlSize = useConsoleSize("L");
  const preference = useContext(ColorSchemeContext);
  const systemScheme = useSyncExternalStore(subscribeColorScheme, systemColorScheme);
  const scheme = preference === "light" || preference === "dark" ? preference : systemScheme;
  const [selectedDate, setSelectedDate] = useState<string>();
  const selected = model.points.find((point) => point.date === selectedDate) ?? model.points[0];
  const shared: ChartProps = {
    data: model.points,
    colorScheme: scheme,
    locale: locale === "en" ? "en-US" : { number: "zh-CN", time: locale === "zh-TW" ? "zh-TW" : "zh-CN" },
    animations: false,
    description: t("home.trend.chartDescription")
  };

  return (
    <Content data-home-trend-chart styles={style({ display: "grid", gap: `[${size(12)}]`, minWidth: 0 })}>
      <Content data-home-trend-plot styles={style({ width: "full", minWidth: 0 })}>
        <Content data-home-trend-scroll styles={style({ width: "full", minWidth: 0, overflowX: "auto", overscrollBehaviorX: "contain" })}>
          <Content styles={style({
            minWidth: { default: `[${size(400)}]`, lg: `[${size(360)}]`, isExtendedRange: `[${size(640)}]` },
            height: `[${size(200)}]`,
            display: "block"
          })({ isExtendedRange: model.points.length > 7 })}>
            {/* Built from the library's chart components. Every bar carries its count. The
                line has no per-point labels: the library routes those through a collision
                pass that hides whichever cannot be placed, and on a 200px tile that was most
                of them, drawn over the line. The rate is read by hover and by the date reader
                below, which is the honest version of what the library offers. */}
            <Chart {...shared} dataTestId="home-combined-chart" height="100%" padding={12}>
              <Bar name="requests" dimension="date" metric="count" metricAxis="yCount" color={{ value: "blue-900" }} paddingRatio={0.4}>
                <BarDirectLabel position="end-outside" format=",.0f" />
                <ChartInspect targets={["item"]}>{(datum) => <Text>{String(datum.summary)}</Text>}</ChartInspect>
              </Bar>
              <Line name="passRate" dimension="date" metric="rate" scaleType="point" color={{ value: "seafoam-900" }} staticPoint="alwaysPoint" showHoverLabel={false}>
                <ChartInspect targets={["item"]}>{(datum) => <Text>{String(datum.summary)}</Text>}</ChartInspect>
              </Line>
              <Axis position="bottom" baseline labels={model.points.map((point) => ({ value: point.date, label: point.axisLabel }))} />
              <Axis position="left" name="yCount" grid ticks numberFormat=",.0f" tickMinStep={5} />
              <Axis position="right" ticks labelFormat="percentage" range={[0, 1]} />
            </Chart>
          </Content>
        </Content>
      </Content>
      <Content
        data-home-trend-legend
        styles={style({ display: "flex", flexWrap: "wrap", alignItems: "center", gap: `[${size(12)}]`, minWidth: 0 })}
      >
        <Content data-home-trend-series="count" styles={style({ display: "flex", alignItems: "center", gap: `[${size(4)}]`, minWidth: 0 })}>
          <Content styles={style({ color: "blue-900" })}><Icon name="chartColumn" /></Content>
          <Text>{t("home.trend.countSeries")}</Text>
        </Content>
        <Content data-home-trend-series="rate" styles={style({ display: "flex", alignItems: "center", gap: `[${size(4)}]`, minWidth: 0 })}>
          <Content styles={style({ color: "seafoam-900" })}><Icon name="chartLine" /></Content>
          <Text>{t("home.trend.rateSeries")}</Text>
        </Content>
      </Content>
      <Content data-home-trend-controls styles={style({ display: "flex", flexWrap: "wrap", alignItems: "center", gap: `[${size(12)}]`, minWidth: 0 })}>
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
  const coverageText = t("home.trend.coverage", { missing: model.missingDays });

  return (
    <Content data-home-section="trend" aria-labelledby="home-trend-title" styles={sectionSurface}>
      <Header data-home-section-heading styles={style({ display: "flex", flexWrap: "wrap", alignItems: "end", justifyContent: "space-between", gap: `[${size(16)}]` })}>
        <Content styles={style({ display: "grid", gap: `[${size(8)}]` })}>
          <Heading level={2} id="home-trend-title" styles={style({ font: "heading", margin: 0 })}>{t("home.trend.title")}</Heading>
          {model.missingDays > 0 ? <Text data-home-trend-coverage styles={style({ font: "body-sm", color: "neutral-subdued" })}>{coverageText}</Text> : null}
        </Content>
        <Link href={`/stats${groupSearch}`} isStandalone isQuiet>
          <Text styles={style({ display: "flex", alignItems: "center", gap: 4, font: "ui-sm", fontWeight: "medium" })}>
            {t("home.trend.openStats")} <Icon name="arrowRight" />
          </Text>
        </Link>
      </Header>
      {model.points.length > 0 ? <TrendChart model={model} locale={i18n.language} /> : (
        <Content data-home-trend-empty styles={style({ display: "flex", alignItems: "center", gap: `[${size(8)}]`, minWidth: 0 })}>
          <Icon name="chartNoAxesCombined" />
          <Text>{t("home.trend.empty")}</Text>
        </Content>
      )}
    </Content>
  );
}
