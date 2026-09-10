import { useTranslation } from "react-i18next";
import { Badge, Content, Heading, Link, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };

import { Icon, type IconName } from "../../icons";
import { useConsoleSession } from "../../app/session";
import { groupName } from "../../lib/chatNames";
import { HomeEntries } from "./HomeEntries";
import { sectionSurface } from "./surface";
import { HomeSourceBadge } from "./HomeSourceBadge";
import type { HomeData } from "./useHomeData";
import { HomeTrend } from "./HomeTrend";

type AttentionTone = "pending" | "error";

type AttentionItem = Readonly<{
  id: string;
  path: string;
  titleKey: string;
  descriptionKey: string;
  tone: AttentionTone;
  count?: number;
}>;

const attentionIcons: Readonly<Record<AttentionTone, IconName>> = {
  pending: "inbox",
  error: "circleAlert"
};

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
      icon: "chartColumn",
      labelKey: "home.overview.challenges",
      value: number.format(data.stats.summary.challenges),
      path: "/stats"
    },
    {
      id: "pass-rate",
      icon: "circleCheck",
      labelKey: "home.overview.passRate",
      value: percent.format(data.stats.summary.pass_rate),
      path: "/stats"
    },
    {
      id: "waiting",
      icon: "inbox",
      labelKey: "home.overview.waiting",
      value: number.format(data.queue.length),
      path: "/queue"
    },
    {
      id: "banned",
      icon: "shieldOff",
      labelKey: "home.overview.banned",
      value: number.format(data.stats.summary.banned),
      path: "/stats"
    }
  ] as const;

  return (
    <Content data-home-section="overview" aria-labelledby="home-overview-title" styles={sectionSurface}>
      <Heading level={2} id="home-overview-title" styles={style({ font: "heading", margin: 0 })}>{t("home.overview.title")}</Heading>
      <Content aria-labelledby="home-overview-title" data-home-metrics styles={style({ display: "grid", gridTemplateColumns: { default: ["minmax(0, 1fr)", "minmax(0, 1fr)"], lg: ["minmax(0, 1fr)", "minmax(0, 1fr)", "minmax(0, 1fr)", "minmax(0, 1fr)"] }, gap: 8, minWidth: 0 })}>
        {metrics.map((metric) => (
          <Link key={metric.id} href={`${metric.path}${groupSearch}`} isStandalone isQuiet data-home-metric={metric.id}>
            <Content styles={style({ display: "grid", gap: 4, minWidth: 0 })}>
              <Text styles={style({ display: "flex", alignItems: "center", gap: 4, font: "ui-sm", color: "neutral-subdued" })}>
                <Icon name={metric.icon} /> {t(metric.labelKey)}
              </Text>
              <Text styles={style({ font: "heading-xl", color: "neutral" })}>{metric.value}</Text>
            </Content>
          </Link>
        ))}
      </Content>
    </Content>
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
    <Content data-home-section="attention" aria-labelledby="home-attention-title" styles={sectionSurface}>
      <Heading level={2} id="home-attention-title" styles={style({ font: "heading", margin: 0 })}>{t("home.attention.title")}</Heading>
      {items.length === 0 ? (
        <Content data-home-attention-empty styles={style({ display: "grid", justifyItems: "start", gap: 8, textAlign: "start" })}>
          <Badge variant="positive" fillStyle="subtle"><Icon name="circleCheck" /> {t("home.attention.empty.badge")}</Badge>
          <Text styles={style({ font: "body", color: "neutral-subdued" })}>{t(isOperator ? "home.attention.empty.operatorDescription" : "home.attention.empty.managerDescription")}</Text>
        </Content>
      ) : (
        <Content aria-labelledby="home-attention-title" data-home-attention-list styles={style({ display: "grid", gap: 8, minWidth: 0 })}>
          {items.map((item) => (
            <Link key={item.id} href={`${item.path}${groupSearch}`} isStandalone isQuiet data-home-attention={item.id}
              aria-label={`${t(item.titleKey)} ${t(item.descriptionKey, { count: item.count })}`}>
              <Content styles={style({ display: "flex", alignItems: "center", gap: 12, font: "body", minWidth: 0, padding: 12, borderRadius: "lg", backgroundColor: "layer-2" })}>
                <Badge variant={item.tone === "error" ? "negative" : "notice"} fillStyle="subtle"><Icon name={attentionIcons[item.tone]} /> {t(`home.attention.tones.${item.tone}`)}</Badge>
                <Text data-home-attention-copy styles={style({ flexGrow: 1 })}>{t(item.titleKey)}</Text>
                {item.count !== undefined ? <Text>{item.count}</Text> : null}
                <Icon name="arrowRight" />
              </Content>
            </Link>
          ))}
        </Content>
      )}
    </Content>
  );
}

export function HomeDashboard({ data, chatID }: Readonly<{ data: HomeData; chatID: string }>) {
  const { t } = useTranslation();
  const session = useConsoleSession();
  const title = session.state === "ready" ? session.chats.find((chat) => chat.id === chatID)?.title : undefined;
  const groupSearch = `?${new URLSearchParams({ group: chatID }).toString()}`;
  const isOperator = data.diagnostics.kind !== "hidden";
  const settings = data.settings;

  return (
    <Content data-home-content styles={style({ display: "grid", minWidth: 0, gap: 8 })}>
      <Content data-home-context aria-labelledby="home-context-title" styles={style({ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 8, minWidth: 0 })}>
        <Content data-home-context-copy>
          <Text slot="title" id="home-context-title">{t("home.context.title", { id: groupName(chatID, title) })}</Text>
        </Content>
        <Content data-home-context-badges styles={style({ display: "flex", flexWrap: "wrap", gap: 8, minWidth: 0 })}>
          <Badge variant="neutral" fillStyle="subtle">
            {t(isOperator ? "home.roles.operator" : "home.roles.manager")}
          </Badge>
          <Badge variant={settings.enabled.value ? "positive" : "neutral"} fillStyle="subtle">
            {t(settings.enabled.value ? "home.context.enabled" : "home.context.disabled")}
          </Badge>
          <HomeSourceBadge source={settings.enabled.source} />
        </Content>
      </Content>
      <Content
        styles={style({
          display: "grid",
          gridTemplateColumns: { default: ["minmax(0, 1fr)"], lg: ["minmax(0, 1fr)", "minmax(0, 1fr)"] },
          gap: 16,
          alignItems: "stretch",
          minWidth: 0
        })}
      >
        <OverviewSection data={data} groupSearch={groupSearch} />
        <AttentionSection data={data} groupSearch={groupSearch} />
        <HomeTrend data={data} groupSearch={groupSearch} />
        <HomeEntries settings={settings} groupSearch={groupSearch} />
      </Content>
    </Content>
  );
}
