import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Card, Content, Heading, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import type { SettingSource } from "../verification/api";
import type { HomeSettings } from "./api";

const verifyModeMessageKeys = {
  kernel: "home.values.verifyMode.kernel",
  quiz: "home.values.verifyMode.quiz",
  mixed: "home.values.verifyMode.mixed"
} as const;

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "home.source.factoryDefault",
  "user file": "home.source.userFile",
  "chat override": "home.source.chatOverride"
};

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
    <Text data-home-entry-value styles={style({ font: "body" })}>
      <Text>{t(labelKey)}</Text>{" "}
      <Text data-home-entry-value-text>{value}</Text>{" "}
      <Text
        data-home-entry-source
        aria-label={t("home.source.value", { source: t(sourceMessageKeys[source]) })}
        styles={style({ font: "body-sm", color: "neutral-subdued" })}
      >
        ({t(sourceMessageKeys[source])})
      </Text>
    </Text>
  );
}

function ConfigEntry({
  id,
  titleKey,
  path,
  groupSearch,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  path: string;
  groupSearch: string;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <Card
      href={`${path}${groupSearch}`}
      data-console-card
      data-home-entry={id}
      styles={style({ width: "full", minWidth: 0 })}
    >
      <Content data-home-entry-values styles={style({ minWidth: 0 })}>
        <Text slot="title">{t(titleKey)}</Text>
        {children}
      </Content>
    </Card>
  );
}

type EntryProps = Readonly<{ settings: HomeSettings; groupSearch: string }>;

function VerificationEntry({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
    <ConfigEntry
      id="verification"
      titleKey="home.entries.verification.title"
      path="/verification"
      groupSearch={groupSearch}
    >
      <ConfigValue
        labelKey="home.entries.verification.mode"
        value={t(verifyModeMessageKeys[settings.verifyMode.value])}
        source={settings.verifyMode.source}
      />
    </ConfigEntry>
  );
}

function QuestionsEntry({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
    <ConfigEntry
      id="questions"
      titleKey="home.entries.questions.title"
      path="/questions"
      groupSearch={groupSearch}
    >
      <ConfigValue
        labelKey="home.entries.questions.primary"
        value={t("home.values.questions", { count: settings.questionCount.value })}
        source={settings.questionCount.source}
      />
    </ConfigEntry>
  );
}

function BypassEntry({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
    <ConfigEntry
      id="bypass"
      titleKey="home.entries.bypass.title"
      path="/bypass"
      groupSearch={groupSearch}
    >
      <ConfigValue
        labelKey="home.entries.bypass.requiredChannel"
        value={
          settings.requiredChannelID.value === 0
            ? t("home.values.disabled")
            : t("home.values.configured")
        }
        source={settings.requiredChannelID.source}
      />
    </ConfigEntry>
  );
}

function ModerationEntry({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
    <ConfigEntry
      id="moderation"
      titleKey="home.entries.moderation.title"
      path="/moderation"
      groupSearch={groupSearch}
    >
      <ConfigValue
        labelKey="home.entries.moderation.antispam"
        value={t(settings.antispamEnabled.value ? "home.values.enabled" : "home.values.disabled")}
        source={settings.antispamEnabled.source}
      />
    </ConfigEntry>
  );
}

export function HomeEntries({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
    <Content
      data-home-section="entries"
      aria-labelledby="home-entries-title"
      styles={style({ display: "grid", gap: 8, minWidth: 0 })}
    >
      <Content data-home-section-heading>
        <Heading level={2} id="home-entries-title" styles={style({ font: "heading", margin: 0 })}>
          {t("home.entries.title")}
        </Heading>
      </Content>
      <Content
        data-home-entries
        styles={style({ display: "grid", gridTemplateColumns: ["minmax(0, 1fr)"], gap: 8, minWidth: 0 })}
      >
        <VerificationEntry settings={settings} groupSearch={groupSearch} />
        <QuestionsEntry settings={settings} groupSearch={groupSearch} />
        <BypassEntry settings={settings} groupSearch={groupSearch} />
        <ModerationEntry settings={settings} groupSearch={groupSearch} />
      </Content>
    </Content>
  );
}
