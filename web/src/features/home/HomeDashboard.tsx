import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import type { ReactNode } from "react";

import { Icon } from "../../icons";
import { StatusBadge, type StatusTone } from "../../components/StatusBadge";
import { HomeEntries } from "./HomeEntries";
import { HomeSourceBadge } from "./HomeSourceBadge";
import type { HomeData } from "./useHomeData";
import { HomeTrend } from "./HomeTrend";




type AttentionItem = Readonly<{
  id: string;
  path: string;
  titleKey: string;
  descriptionKey: string;
  tone: StatusTone;
  count?: number;
}>;


function DashboardLink({
  path,
  groupSearch,
  children,
  ...attributes
}: Readonly<{
  path: string;
  groupSearch: string;
  children: ReactNode;
  "data-home-metric"?: string;
  "data-home-attention"?: string;
}>) {
  return (
    <Link
      to={{ pathname: path, search: groupSearch }}
      data-console-card
      {...attributes}
    >
      {children}
    </Link>
  );
}




function attentionItems(data: HomeData): readonly AttentionItem[] {
  const items: AttentionItem[] = [];
  if (data.queue.length > 0) {
    items.push({
      id: "queue",
      path: "/queue",
      titleKey: "home.attention.queue.title",
      descriptionKey: "home.attention.queue.description",
      tone: "pending",
      count: data.queue.length
    });
  }

  if (data.diagnostics.kind === "unavailable") {
    items.push({
      id: "diagnostics-unavailable",
      path: "/diagnostics",
      titleKey: "home.attention.diagnosticsUnavailable.title",
      descriptionKey: "home.attention.diagnosticsUnavailable.description",
      tone: "error"
    });
  }
  if (data.diagnostics.kind !== "loaded") {
    return items;
  }

  const { health, persistence } = data.diagnostics.diagnostics;
  if (!health.live || !health.ready || !health.configReady || !health.telegramReady) {
    items.push({
      id: "health",
      path: "/diagnostics",
      titleKey: "home.attention.health.title",
      descriptionKey: "home.attention.health.description",
      tone: "error"
    });
  }
  if (persistence.lastError !== null) {
    items.push({
      id: "persistence-error",
      path: "/diagnostics",
      titleKey: "home.attention.persistenceError.title",
      descriptionKey: "home.attention.persistenceError.description",
      tone: "error"
    });
  } else if (!persistence.configured) {
    items.push({
      id: "persistence-unconfigured",
      path: "/diagnostics",
      titleKey: "home.attention.persistenceUnconfigured.title",
      descriptionKey: "home.attention.persistenceUnconfigured.description",
      tone: "pending"
    });
  } else if (!persistence.writable) {
    items.push({
      id: "persistence-unwritable",
      path: "/diagnostics",
      titleKey: "home.attention.persistenceUnwritable.title",
      descriptionKey: "home.attention.persistenceUnwritable.description",
      tone: "error"
    });
  } else if (!persistence.durable) {
    items.push({
      id: "persistence-volatile",
      path: "/diagnostics",
      titleKey: "home.attention.persistenceVolatile.title",
      descriptionKey: "home.attention.persistenceVolatile.description",
      tone: "pending"
    });
  }

  return items;
}

function OverviewSection({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { t, i18n } = useTranslation();
  const number = new Intl.NumberFormat(i18n.language);
  const percent = new Intl.NumberFormat(i18n.language, {
    style: "percent",
    maximumFractionDigits: 1
  });
  const metrics = [
    {
      id: "challenges",
      labelKey: "home.overview.challenges",
      value: number.format(data.stats.summary.challenges),
      path: "/stats"
    },
    {
      id: "pass-rate",
      labelKey: "home.overview.passRate",
      value: percent.format(data.stats.summary.pass_rate),
      path: "/stats"
    },
    {
      id: "waiting",
      labelKey: "home.overview.waiting",
      value: number.format(data.queue.length),
      path: "/queue"
    },
    {
      id: "banned",
      labelKey: "home.overview.banned",
      value: number.format(data.stats.summary.banned),
      path: "/stats"
    }
  ] as const;

  return (
    <section data-home-section="overview" aria-labelledby="home-overview-title">
      <header data-home-section-heading>
        <span>
          <h2 id="home-overview-title">{t("home.overview.title")}</h2>
          <p>{t("home.overview.description")}</p>
        </span>
      </header>
      <div data-home-metrics>
        {metrics.map((metric) => (
          <DashboardLink
            key={metric.id}
            path={metric.path}
            groupSearch={groupSearch}
            data-home-metric={metric.id}
          >
            <strong>{metric.value}</strong>
            <span>{t(metric.labelKey)}</span>
          </DashboardLink>
        ))}
      </div>
    </section>
  );
}

function AttentionSection({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { t } = useTranslation();
  const items = attentionItems(data);
  const isOperator = data.diagnostics.kind !== "hidden";

  return (
    <section data-home-section="attention" aria-labelledby="home-attention-title">
      <header data-home-section-heading>
        <span>
          <h2 id="home-attention-title">{t("home.attention.title")}</h2>
          <p>{t("home.attention.description")}</p>
        </span>
      </header>
      {items.length === 0 ? (
        <div data-console-card data-home-attention-empty>
          <StatusBadge tone="ok">{t("home.attention.empty.badge")}</StatusBadge>
          <p>
            {t(
              isOperator
                ? "home.attention.empty.operatorDescription"
                : "home.attention.empty.managerDescription"
            )}
          </p>
        </div>
      ) : (
        <div data-home-attention-list>
          {items.map((item) => (
            <DashboardLink
              key={item.id}
              path={item.path}
              groupSearch={groupSearch}
              data-home-attention={item.id}
            >
              <StatusBadge tone={item.tone}>{t(`home.attention.tones.${item.tone}`)}</StatusBadge>
              <span data-home-attention-copy>
                <strong>{t(item.titleKey)}</strong>
                <span>{t(item.descriptionKey, { count: item.count })}</span>
              </span>
              <Icon name="arrowRight" aria-hidden="true" />
            </DashboardLink>
          ))}
        </div>
      )}
    </section>
  );
}


export function HomeDashboard({ data, chatID }: Readonly<{ data: HomeData; chatID: string }>) {
  const { t } = useTranslation();
  const groupSearch = `?${new URLSearchParams({ group: chatID }).toString()}`;
  const isOperator = data.diagnostics.kind !== "hidden";
  const settings = data.settings;

  return (
    <div data-home-content>
      <aside data-console-card data-home-context aria-labelledby="home-context-title">
        <span data-home-context-copy>
          <strong id="home-context-title">{t("home.context.title", { id: chatID })}</strong>
          <span>{t("home.context.selectedScope")}</span>
        </span>
        <span data-home-context-badges>
          <StatusBadge tone="neutral">
            {t(isOperator ? "home.roles.operator" : "home.roles.manager")}
          </StatusBadge>
          <StatusBadge tone={settings.enabled.value ? "ok" : "neutral"}>
            {t(settings.enabled.value ? "home.context.enabled" : "home.context.disabled")}
          </StatusBadge>
          <HomeSourceBadge source={settings.enabled.source} />
          <StatusBadge tone="neutral">{t("home.context.modeUnreported")}</StatusBadge>
        </span>
      </aside>
      <OverviewSection data={data} groupSearch={groupSearch} />
      <AttentionSection data={data} groupSearch={groupSearch} />
      <HomeTrend data={data} groupSearch={groupSearch} />
      <HomeEntries settings={settings} groupSearch={groupSearch} />
    </div>
  );
}
