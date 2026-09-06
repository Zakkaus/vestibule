import type { ReactNode } from "react";
import {
  Button,
  Card,
  Group,
  Paper,
  SimpleGrid,
  Stack,
  Text,
  Title
} from "@mantine/core";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import { useConsoleSession } from "../../app/session";
import { StatusBadge, type StatusTone } from "../../components/StatusBadge";
import { Icon, type IconName } from "../../icons";
import type { HomeData, HomeDataState } from "./useHomeData";
import { useHomeData } from "./useHomeData";
import { HomeEntries, SourceBadge } from "./HomeEntries";
import { HomeTrend } from "./HomeTrend";



type AttentionItem = Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  tone: StatusTone;
  count?: number;
}>;



function StateCard({
  id,
  icon,
  titleKey,
  descriptionKey,
  role,
  live,
  children
}: Readonly<{
  id: string;
  icon: IconName;
  titleKey: string;
  descriptionKey: string;
  role?: "alert";
  live?: "polite";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <Card
      component="section"
      withBorder
      data-home-state-card={id}
      role={role}
      aria-live={live}
      aria-labelledby={`home-${id}-title`}
      p="xl"
    >
      <Stack gap="md" align="flex-start">
        <Group gap="xs">
          <Icon name={icon} />
          <Title order={2} size="h3" id={`home-${id}-title`}>
            {t(titleKey)}
          </Title>
        </Group>
        <Text c="dimmed">{t(descriptionKey)}</Text>
        {children}
      </Stack>
    </Card>
  );
}


function attentionItems(data: HomeData): readonly AttentionItem[] {
  const items: AttentionItem[] = [];
  if (data.queue.length > 0) {
    items.push({
      id: "queue",
      titleKey: "home.attention.queue.title",
      descriptionKey: "home.attention.queue.description",
      tone: "pending",
      count: data.queue.length
    });
  }

  if (data.diagnostics.kind === "unavailable") {
    items.push({
      id: "diagnostics-unavailable",
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
      titleKey: "home.attention.health.title",
      descriptionKey: "home.attention.health.description",
      tone: "error"
    });
  }
  if (persistence.lastError !== null) {
    items.push({
      id: "persistence-error",
      titleKey: "home.attention.persistenceError.title",
      descriptionKey: "home.attention.persistenceError.description",
      tone: "error"
    });
  } else if (!persistence.configured) {
    items.push({
      id: "persistence-unconfigured",
      titleKey: "home.attention.persistenceUnconfigured.title",
      descriptionKey: "home.attention.persistenceUnconfigured.description",
      tone: "pending"
    });
  } else if (!persistence.writable) {
    items.push({
      id: "persistence-unwritable",
      titleKey: "home.attention.persistenceUnwritable.title",
      descriptionKey: "home.attention.persistenceUnwritable.description",
      tone: "error"
    });
  } else if (!persistence.durable) {
    items.push({
      id: "persistence-volatile",
      titleKey: "home.attention.persistenceVolatile.title",
      descriptionKey: "home.attention.persistenceVolatile.description",
      tone: "pending"
    });
  }

  return items;
}

function SectionHeading({
  id,
  titleKey,
  descriptionKey
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
}>) {
  const { t } = useTranslation();
  return (
    <Stack component="header" gap="xs">
      <Title order={2} size="h3" id={id}>
        {t(titleKey)}
      </Title>
      <Text c="dimmed">{t(descriptionKey)}</Text>
    </Stack>
  );
}

function OverviewSection({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { i18n, t } = useTranslation();
  const number = new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language);
  const percent = new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language, {
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
    <Card component="section" withBorder data-home-section="overview" p="lg">
      <Stack gap="lg">
        <SectionHeading
          id="home-overview-title"
          titleKey="home.overview.title"
          descriptionKey="home.overview.description"
        />
        <SimpleGrid cols={{ base: 1, sm: 2, lg: 4 }} spacing="md">
          {metrics.map((metric) => (
            <Card
              key={metric.id}
              component={Link}
              to={{ pathname: metric.path, search: groupSearch }}
              withBorder
              p="lg"
              data-home-metric={metric.id}
            >
              <Stack gap="xs">
                <Text size="xl" fw={700} lh={1.2}>
                  {metric.value}
                </Text>
                <Text size="sm" c="dimmed">
                  {t(metric.labelKey)}
                </Text>
              </Stack>
            </Card>
          ))}
        </SimpleGrid>
      </Stack>
    </Card>
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
    <Card component="section" withBorder data-home-section="attention" p="lg">
      <Stack gap="lg">
        <SectionHeading
          id="home-attention-title"
          titleKey="home.attention.title"
          descriptionKey="home.attention.description"
        />
        {items.length === 0 ? (
          <Group gap="sm" align="flex-start" data-home-attention-empty>
            <StatusBadge presentation="library" tone="ok">{t("home.attention.empty.badge")}</StatusBadge>
            <Text>
              {t(
                isOperator
                  ? "home.attention.empty.operatorDescription"
                  : "home.attention.empty.managerDescription"
              )}
            </Text>
          </Group>
        ) : (
          <Stack gap="sm" data-home-attention-list>
            {items.map((item) => (
              <Card
                key={item.id}
                component={Link}
                to={{
                  pathname: item.id === "queue" ? "/queue" : "/diagnostics",
                  search: groupSearch
                }}
                withBorder
                p="md"
                data-home-attention={item.id}
              >
                <Stack gap="sm">
                  <Group justify="space-between" align="center" wrap="nowrap">
                    <StatusBadge presentation="library" tone={item.tone}>{t(`home.attention.tones.${item.tone}`)}</StatusBadge>
                    <Icon name="arrowRight" />
                  </Group>
                  <Stack gap="xs">
                    <Text fw={600}>{t(item.titleKey)}</Text>
                    <Text c="dimmed">{t(item.descriptionKey, { count: item.count })}</Text>
                  </Stack>
                </Stack>
              </Card>
            ))}
          </Stack>
        )}
      </Stack>
    </Card>
  );
}


function LoadedHome({ data, chatID }: Readonly<{ data: HomeData; chatID: string }>) {
  const { t } = useTranslation();
  const groupSearch = `?${new URLSearchParams({ group: chatID }).toString()}`;
  const isOperator = data.diagnostics.kind !== "hidden";

  return (
    <Stack gap="xl" data-home-content>
      <Paper component="aside" withBorder data-home-context p="lg" aria-labelledby="home-context-title">
        <Group justify="space-between" align="center" wrap="wrap" gap="md">
          <Stack gap="xs">
            <Title order={2} size="h3" id="home-context-title">
              {t("home.context.title", { id: chatID })}
            </Title>
            <Text c="dimmed">{t("home.context.selectedScope")}</Text>
          </Stack>
          <Group gap="xs" wrap="wrap">
            <StatusBadge presentation="library" tone="neutral">{t(isOperator ? "home.roles.operator" : "home.roles.manager")}</StatusBadge>
            <StatusBadge presentation="library" tone={data.settings.enabled.value ? "ok" : "neutral"}>{t(data.settings.enabled.value ? "home.context.enabled" : "home.context.disabled")}</StatusBadge>
            <SourceBadge source={data.settings.enabled.source} />
            <StatusBadge presentation="library" tone="neutral">{t("home.context.modeUnreported")}</StatusBadge>
          </Group>
        </Group>
      </Paper>
      <OverviewSection data={data} groupSearch={groupSearch} />
      <AttentionSection data={data} groupSearch={groupSearch} />
      <HomeTrend data={data} groupSearch={groupSearch} />
      <HomeEntries data={data} groupSearch={groupSearch} />
    </Stack>
  );
}

function HomeStateContent({
  state,
  chatID,
  reload
}: Readonly<{
  state: HomeDataState;
  chatID: string | undefined;
  reload: () => void;
}>) {
  const { t } = useTranslation();
  if (state.kind === "loaded" && chatID) {
    return <LoadedHome data={state.data} chatID={chatID} />;
  }
  if (state.kind === "loading") {
    return (
      <StateCard
        id="loading"
        icon="loaderCircle"
        titleKey="home.loading.title"
        descriptionKey="home.loading.description"
        live="polite"
      />
    );
  }
  if (state.kind === "group-required" || (state.kind === "loaded" && !chatID)) {
    return (
      <StateCard
        id="group-required"
        icon="usersRound"
        titleKey="home.groupRequired.title"
        descriptionKey="home.groupRequired.description"
      >
        <Button
          component={Link}
          to="/groups"
          variant="filled"
          size="sm"
          leftSection={<Icon name="usersRound" />}
        >
          {t("home.groupRequired.select")}
        </Button>
      </StateCard>
    );
  }
  if (state.kind === "no-groups") {
    return (
      <StateCard
        id="no-groups"
        icon="usersRound"
        titleKey="home.noGroups.title"
        descriptionKey="home.noGroups.description"
      />
    );
  }
  return (
    <StateCard
      id="unavailable"
      icon="circleAlert"
      titleKey="home.unavailable.title"
      descriptionKey="home.unavailable.description"
      role="alert"
    >
      <Button
        type="button"
        variant="default"
        size="sm"
        onClick={reload}
        leftSection={<Icon name="refreshCw" />}
      >
        {t("home.unavailable.retry")}
      </Button>
    </StateCard>
  );
}

export function HomeScreen() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const session = useConsoleSession();
  const chatID = searchParams.get("group") ?? undefined;
  const controller = useHomeData(session, chatID);

  return (
    <Stack
      component="section"
      gap="xl"
      data-home-page
      data-home-state={controller.state.kind}
      aria-busy={controller.state.kind === "loading" || undefined}
      aria-labelledby="home-title"
    >
      <Stack component="header" gap="xs">
        <Title order={1} id="home-title">
          {t("home.title")}
        </Title>
        <Text c="dimmed">{t("home.description")}</Text>
      </Stack>
      <HomeStateContent state={controller.state} chatID={chatID} reload={controller.reload} />
    </Stack>
  );
}
