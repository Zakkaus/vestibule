import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import type { StatsDay } from "../stats/api";
import type { HomeData } from "./useHomeData";

type TrendChartPoint = Readonly<{
  day: StatsDay;
  x: number;
  barX: number;
  barWidth: number;
  barY: number;
  barHeight: number;
  passRateY: number;
}>;

function trendChartPoints(trend: readonly StatsDay[]): Readonly<{
  points: readonly TrendChartPoint[];
  passRatePath: string;
}> {
  const plotLeft = 28;
  const plotRight = 672;
  const plotTop = 16;
  const plotBottom = 132;
  const plotHeight = plotBottom - plotTop;
  const step = (plotRight - plotLeft) / Math.max(1, trend.length);
  const barWidth = Math.max(1, Math.min(34, step * 0.5));
  const maxChallenges = Math.max(1, ...trend.map((day) => day.challenges));
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
      passRateY: plotBottom - day.pass_rate * plotHeight
    };
  });
  const passRatePath = points
    .map((point, index) => `${index === 0 ? "M" : "L"} ${point.x} ${point.passRateY}`)
    .join(" ");
  return { points, passRatePath };
}

export function HomeTrend({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { t } = useTranslation();
  const chart = trendChartPoints(data.stats.trend);

  return (
    <section data-slot="card" data-home-section="trend" aria-labelledby="home-trend-title">
      <header data-home-section-heading>
        <span data-home-section-copy>
          <h2 id="home-trend-title">{t("home.trend.title")}</h2>
          <p>{t("home.trend.description")}</p>
        </span>
        <Link
          to={{ pathname: "/stats", search: groupSearch }}
          data-slot="button"
          data-variant="link"
          data-size="sm"
        >
          {t("home.trend.openStats")}
        </Link>
      </header>
      {chart.points.length === 0 ? (
        <p data-home-trend-empty>{t("home.trend.empty")}</p>
      ) : (
        <>
          <svg
            data-home-trend-chart
            viewBox="0 0 700 148"
            role="img"
            aria-labelledby="home-trend-chart-title home-trend-chart-description"
          >
            <title id="home-trend-chart-title">{t("home.trend.chartTitle")}</title>
            <desc id="home-trend-chart-description">{t("home.trend.chartDescription")}</desc>
            <line x1="28" y1="132" x2="672" y2="132" stroke="var(--border)" />
            {chart.points.map((point) => (
              <rect
                key={point.day.date}
                data-home-chart-bar
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
              <circle
                key={`${point.day.date}-rate`}
                cx={point.x}
                cy={point.passRateY}
                r="3"
                fill="var(--card)"
                stroke="var(--fill-ok)"
                strokeWidth="2"
              />
            ))}
          </svg>
          <div data-home-trend-legend aria-label={t("home.trend.legendLabel")}>
            <span data-series="challenges">{t("home.trend.challenges")}</span>
            <span data-series="pass-rate">{t("home.trend.passRate")}</span>
          </div>
        </>
      )}
    </section>
  );
}
