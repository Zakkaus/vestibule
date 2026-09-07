import { useTranslation } from "react-i18next";
import { Badge, Card, Content, Divider, Header, Heading, IllustratedMessage, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };

import type { StatusTone } from "../../components/StatusBadge";
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
    <Content data-home-section="overview" aria-labelledby="home-overview-title" styles={style({ display: "grid", gap: 16, minWidth: 0 })}>
      <Divider size="S" />
      <Header data-home-section-heading styles={style({ display: "grid", gap: 8 })}>
        <Heading level={2} id="home-overview-title" styles={style({ font: "heading", margin: 0 })}>{t("home.overview.title")}</Heading>
        <Text styles={style({ font: "body", color: "neutral-subdued" })}>{t("home.overview.description")}</Text>
      </Header>
      <Content aria-labelledby="home-overview-title" data-home-metrics styles={style({ display: "grid", gridTemplateColumns: { default: ["minmax(0, 1fr)", "minmax(0, 1fr)"], lg: ["minmax(0, 1fr)", "minmax(0, 1fr)", "minmax(0, 1fr)", "minmax(0, 1fr)"] }, gap: 16, minWidth: 0 })}>
        {metrics.map((metric) => (
          <Card key={metric.id} href={`${metric.path}${groupSearch}`} density="compact" data-console-card data-home-metric={metric.id} styles={style({ width: "full", minWidth: 0 })}>
            <Content>
              <Text slot="title" styles={style({ font: "heading-lg" })}>{metric.value}</Text>
              <Text slot="description">{t(metric.labelKey)}</Text>
            </Content>
          </Card>
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
    <Content data-home-section="attention" aria-labelledby="home-attention-title" styles={style({ display: "grid", gap: 16, minWidth: 0 })}>
      <Divider size="S" />
      <Header data-home-section-heading styles={style({ display: "grid", gap: 8 })}>
        <Heading level={2} id="home-attention-title" styles={style({ font: "heading", margin: 0 })}>{t("home.attention.title")}</Heading>
        <Text styles={style({ font: "body", color: "neutral-subdued" })}>{t("home.attention.description")}</Text>
      </Header>
      {items.length === 0 ? (
        <IllustratedMessage data-home-attention-empty size="S">
          <Heading>{t("home.attention.empty.badge")}</Heading>
          <Content>{t(isOperator ? "home.attention.empty.operatorDescription" : "home.attention.empty.managerDescription")}</Content>
        </IllustratedMessage>
      ) : (
        <Content aria-labelledby="home-attention-title" data-home-attention-list styles={style({ display: "grid", gap: 16, minWidth: 0 })}>
          {items.map((item) => (
            <Card key={item.id} href={`${item.path}${groupSearch}`} data-console-card data-home-attention={item.id} styles={style({ width: "full", minWidth: 0 })}>
              <Badge variant={item.tone === "error" ? "negative" : "notice"} fillStyle="subtle">{t(`home.attention.tones.${item.tone}`)}</Badge>
              <Content data-home-attention-copy>
                <Text slot="title">{t(item.titleKey)}</Text>
                <Text slot="description">{t(item.descriptionKey, { count: item.count })}</Text>
              </Content>
            </Card>
          ))}
        </Content>
      )}
    </Content>
  );
}


export function HomeDashboard({ data, chatID }: Readonly<{ data: HomeData; chatID: string }>) {
  const { t } = useTranslation();
  const groupSearch = `?${new URLSearchParams({ group: chatID }).toString()}`;
  const isOperator = data.diagnostics.kind !== "hidden";
  const settings = data.settings;

  return (
    <Content data-home-content styles={style({ display: "grid", minWidth: 0, gap: 24 })}>
      <Card data-console-card data-home-context aria-labelledby="home-context-title" styles={style({ width: "full", minWidth: 0 })}>
        <Content data-home-context-copy>
          <Text slot="title" id="home-context-title">{t("home.context.title", { id: chatID })}</Text>
          <Text slot="description">{t("home.context.selectedScope")}</Text>
        </Content>
        <Content data-home-context-badges styles={style({ display: "flex", flexWrap: "wrap", gap: 8, minWidth: 0 })}>
          <Badge variant="neutral" fillStyle="subtle">
            {t(isOperator ? "home.roles.operator" : "home.roles.manager")}
          </Badge>
          <Badge variant={settings.enabled.value ? "positive" : "neutral"} fillStyle="subtle">
            {t(settings.enabled.value ? "home.context.enabled" : "home.context.disabled")}
          </Badge>
          <HomeSourceBadge source={settings.enabled.source} />
          <Badge variant="neutral" fillStyle="subtle">{t("home.context.modeUnreported")}</Badge>
        </Content>
      </Card>
      <OverviewSection data={data} groupSearch={groupSearch} />
      <AttentionSection data={data} groupSearch={groupSearch} />
      <HomeTrend data={data} groupSearch={groupSearch} />
      <HomeEntries settings={settings} groupSearch={groupSearch} />
    </Content>
  );
}
