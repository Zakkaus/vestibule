import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Card, Content, Divider, Footer, Header, Heading, LabeledValue, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import type { SettingSource } from "../verification/api";
import type { HomeSettings } from "./api";
import { HomeSourceBadge } from "./HomeSourceBadge";

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
    <LabeledValue
      data-home-entry-value
      label={t(labelKey)}
      value={
        <Content styles={style({ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 8 })}>
          <Text>{value}</Text>
          <HomeSourceBadge source={source} />
        </Content>
      }
    />
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
    <Card href={`${path}${groupSearch}`} data-console-card data-home-entry={id} styles={style({ width: "full", minWidth: 0 })}>
      <Content data-home-entry-heading>
        <Text slot="title">{t(titleKey)}</Text>
        <Text slot="description">{t(descriptionKey)}</Text>
      </Content>
      <Divider size="S" />
      <Footer data-home-entry-values styles={style({ display: "grid", gap: 16, minWidth: 0 })}>{children}</Footer>
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
  );
}

function QuestionsEntry({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
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
  );
}

function BypassEntry({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
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
  );
}

function ModerationEntry({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
    <ConfigEntry
      id="moderation"
      titleKey="home.entries.moderation.title"
      descriptionKey="home.entries.moderation.description"
      path="/moderation"
      groupSearch={groupSearch}
    >
      <ConfigValue
        labelKey="home.entries.moderation.antispam"
        value={t(settings.antispamEnabled.value ? "home.values.enabled" : "home.values.disabled")}
        source={settings.antispamEnabled.source}
      />
      <ConfigValue
        labelKey="home.entries.moderation.warnLimit"
        value={t("home.values.warnings", { count: settings.warnLimit.value })}
        source={settings.warnLimit.source}
      />
    </ConfigEntry>
  );
}

export function HomeEntries({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
    <Content data-home-section="entries" aria-labelledby="home-entries-title" styles={style({ display: "grid", gap: 16, minWidth: 0 })}>
      <Divider size="S" />
      <Header data-home-section-heading styles={style({ display: "grid", gap: 8 })}>
        <Heading level={2} id="home-entries-title" styles={style({ font: "heading", margin: 0 })}>{t("home.entries.title")}</Heading>
        <Text styles={style({ font: "body", color: "neutral-subdued" })}>{t("home.entries.description")}</Text>
      </Header>
      <Content data-home-entries styles={style({ display: "grid", gridTemplateColumns: { default: ["minmax(0, 1fr)"], md: ["minmax(0, 1fr)", "minmax(0, 1fr)"] }, gap: 16 })}>
        <VerificationEntry settings={settings} groupSearch={groupSearch} />
        <QuestionsEntry settings={settings} groupSearch={groupSearch} />
        <BypassEntry settings={settings} groupSearch={groupSearch} />
        <ModerationEntry settings={settings} groupSearch={groupSearch} />
      </Content>
    </Content>
  );
}
