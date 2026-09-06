import type { ReactNode } from "react";
import {
  Badge,
  Card,
  Group,
  SimpleGrid,
  Stack,
  Text,
  Title
} from "@mantine/core";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import { Icon } from "../../icons";
import type { SettingSource } from "../verification/api";
import type { HomeData } from "./useHomeData";

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "home.source.factoryDefault",
  "user file": "home.source.userFile",
  "chat override": "home.source.chatOverride"
};

const verifyModeMessageKeys = {
  kernel: "home.values.verifyMode.kernel",
  quiz: "home.values.verifyMode.quiz",
  mixed: "home.values.verifyMode.mixed"
} as const;

const deliveryModeMessageKeys = {
  group: "home.values.deliveryMode.group",
  dm: "home.values.deliveryMode.dm",
  both: "home.values.deliveryMode.both"
} as const;

export function SourceBadge({ source }: Readonly<{ source: SettingSource }>) {
  const { t } = useTranslation();
  return (
    <Badge size="sm" variant="default" data-home-source={source}>
      {t("home.source.value", { source: t(sourceMessageKeys[source]) })}
    </Badge>
  );
}

function ConfigValue({
  labelKey,
  value,
  source
}: Readonly<{
  labelKey: string;
  value: string;
  source: SettingSource;
}>) {
  const { t } = useTranslation();
  return (
    <Stack gap="xs" data-home-entry-value>
      <Text component="dt" size="sm" c="dimmed">
        {t(labelKey)}
      </Text>
      <Group component="dd" m={0} gap="xs" align="center">
        <Text component="span" fw={500}>
          {value}
        </Text>
        <SourceBadge source={source} />
      </Group>
    </Stack>
  );
}

function ConfigEntry({
  id,
  titleKey,
  descriptionKey,
  path,
  groupSearch,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  path: string;
  groupSearch: string;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <Card
      component={Link}
      to={{ pathname: path, search: groupSearch }}
      withBorder
      p="lg"
      data-home-entry={id}
    >
      <Stack gap="md">
        <Group justify="space-between" align="center" wrap="nowrap">
          <Title order={3} size="h4">
            {t(titleKey)}
          </Title>
          <Icon name="arrowRight" />
        </Group>
        <Text c="dimmed">{t(descriptionKey)}</Text>
        <Stack component="dl" gap="sm">
          {children}
        </Stack>
      </Stack>
    </Card>
  );
}

export function HomeEntries({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { t } = useTranslation();
  const { settings } = data;

  return (
    <Card component="section" withBorder data-home-section="entries" p="lg">
      <Stack gap="lg">
        <Stack component="header" gap="xs">
          <Title order={2} size="h3" id="home-entries-title">
            {t("home.entries.title")}
          </Title>
          <Text c="dimmed">{t("home.entries.description")}</Text>
        </Stack>
        <SimpleGrid cols={{ base: 1, md: 2 }} spacing="md" data-home-entries>
          <ConfigEntry
            id="verification"
            titleKey="home.entries.verification.title"
            descriptionKey="home.entries.verification.description"
            path="/verification"
            groupSearch={groupSearch}
          >
            <ConfigValue
              labelKey="home.entries.verification.mode"
              value={t(verifyModeMessageKeys[settings.verifyMode.value])}
              source={settings.verifyMode.source}
            />
            <ConfigValue
              labelKey="home.entries.verification.delivery"
              value={t(deliveryModeMessageKeys[settings.deliveryMode.value])}
              source={settings.deliveryMode.source}
            />
            <ConfigValue
              labelKey="home.entries.verification.timeout"
              value={t("home.values.seconds", { count: settings.timeoutSeconds.value })}
              source={settings.timeoutSeconds.source}
            />
          </ConfigEntry>
          <ConfigEntry
            id="questions"
            titleKey="home.entries.questions.title"
            descriptionKey="home.entries.questions.description"
            path="/questions"
            groupSearch={groupSearch}
          >
            <ConfigValue
              labelKey="home.entries.questions.primary"
              value={t("home.values.rules", { count: settings.questionCount.value })}
              source={settings.questionCount.source}
            />
            <ConfigValue
              labelKey="home.entries.questions.fallback"
              value={t("home.values.rules", { count: settings.fallbackQuestionCount.value })}
              source={settings.fallbackQuestionCount.source}
            />
          </ConfigEntry>
          <ConfigEntry
            id="bypass"
            titleKey="home.entries.bypass.title"
            descriptionKey="home.entries.bypass.description"
            path="/bypass"
            groupSearch={groupSearch}
          >
            <ConfigValue
              labelKey="home.entries.bypass.trustedGroups"
              value={t("home.values.groups", { count: settings.trustedGroupCount.value })}
              source={settings.trustedGroupCount.source}
            />
            <ConfigValue
              labelKey="home.entries.bypass.channels"
              value={t("home.values.channels", { count: settings.channelWhitelistCount.value })}
              source={settings.channelWhitelistCount.source}
            />
          </ConfigEntry>
          <ConfigEntry
            id="moderation"
            titleKey="home.entries.moderation.title"
            descriptionKey="home.entries.moderation.description"
            path="/moderation"
            groupSearch={groupSearch}
          >
            <ConfigValue
              labelKey="home.entries.moderation.antispam"
              value={t(
                settings.antispamEnabled.value ? "home.values.enabled" : "home.values.disabled"
              )}
              source={settings.antispamEnabled.source}
            />
            <ConfigValue
              labelKey="home.entries.moderation.warnLimit"
              value={t("home.values.warnings", { count: settings.warnLimit.value })}
              source={settings.warnLimit.source}
            />
          </ConfigEntry>
        </SimpleGrid>
      </Stack>
    </Card>
  );
}
