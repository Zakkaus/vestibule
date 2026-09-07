import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Card, Content, Divider, Footer, Heading, LabeledValue, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import type { TFunction } from "i18next";
import type { SettingSource } from "../verification/api";
import type { HomeSettings } from "./api";

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

const languageMessageKeys = {
  zh: "questions.language.zh",
  "zh-Hant": "questions.language.zhHant",
  en: "questions.language.en"
} as const;

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "home.source.factoryDefault",
  "user file": "home.source.userFile",
  "chat override": "home.source.chatOverride"
};

function durationMessage(t: TFunction, seconds: number): string {
  return t("home.values.seconds", { count: seconds });
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
    <LabeledValue
      data-home-entry-value
      label={t(labelKey)}
      value={
        <Text styles={style({ display: "grid", gap: 4 })}>
          <Text data-home-entry-value-text>{value}</Text>
          <Text
            data-home-entry-source
            styles={style({ font: "body-sm", color: "neutral-subdued" })}
          >
            {t("home.source.value", { source: t(sourceMessageKeys[source]) })}
          </Text>
        </Text>
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
    <Card
      href={`${path}${groupSearch}`}
      data-console-card
      data-home-entry={id}
      styles={style({ width: "full", minWidth: 0 })}
    >
      <Content data-home-entry-heading>
        <Text slot="title">{t(titleKey)}</Text>
        <Text slot="description">{t(descriptionKey)}</Text>
      </Content>
      <Divider size="S" />
      <Footer
        data-home-entry-values
        styles={style({ display: "grid", gap: 16, minWidth: 0 })}
      >
        {children}
      </Footer>
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
        value={durationMessage(t, settings.timeoutSeconds.value)}
        source={settings.timeoutSeconds.source}
      />
      <ConfigValue
        labelKey="home.entries.verification.maxFails"
        value={
          settings.verifyMaxFails.value > 0
            ? t("home.values.failures", { count: settings.verifyMaxFails.value })
            : t("home.values.disabled")
        }
        source={settings.verifyMaxFails.source}
      />
      <ConfigValue
        labelKey="home.entries.verification.retry"
        value={
          settings.verifyRetrySeconds.value > 0
            ? durationMessage(t, settings.verifyRetrySeconds.value)
            : t("home.values.disabled")
        }
        source={settings.verifyRetrySeconds.source}
      />
      <ConfigValue
        labelKey="home.entries.verification.ban"
        value={
          settings.banSeconds.value === 0
            ? t("home.values.permanent")
            : durationMessage(t, settings.banSeconds.value)
        }
        source={settings.banSeconds.source}
      />
      <ConfigValue
        labelKey="home.entries.verification.mute"
        value={durationMessage(t, settings.muteSeconds.value)}
        source={settings.muteSeconds.source}
      />
      <ConfigValue
        labelKey="home.entries.verification.invited"
        value={t(settings.verifyInvited.value ? "home.values.enabled" : "home.values.disabled")}
        source={settings.verifyInvited.source}
      />
      <ConfigValue
        labelKey="home.entries.verification.nameSpoiler"
        value={t(settings.nameSpoiler.value ? "home.values.enabled" : "home.values.disabled")}
        source={settings.nameSpoiler.source}
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
        value={t("home.values.questions", { count: settings.questionCount.value })}
        source={settings.questionCount.source}
      />
      <ConfigValue
        labelKey="home.entries.questions.fallback"
        value={t("home.values.questions", { count: settings.fallbackQuestionCount.value })}
        source={settings.fallbackQuestionCount.source}
      />
      <ConfigValue
        labelKey="home.entries.questions.mode"
        value={t(settings.fallbackBuiltin.value ? "questions.fallback.builtin" : "questions.fallback.custom")}
        source={settings.fallbackBuiltin.source}
      />
      <ConfigValue
        labelKey="home.entries.questions.language"
        value={t(languageMessageKeys[settings.questionLanguage.value])}
        source={settings.questionLanguage.source}
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
        labelKey="home.entries.bypass.requiredChannel"
        value={
          settings.requiredChannelID.value === 0
            ? t("home.values.disabled")
            : t("home.values.configured")
        }
        source={settings.requiredChannelID.source}
      />
      <ConfigValue
        labelKey="home.entries.bypass.channelDisplay"
        value={settings.channelDisplay.value || t("home.values.notConfigured")}
        source={settings.channelDisplay.source}
      />
      <ConfigValue
        labelKey="home.entries.bypass.channelInvite"
        value={settings.channelInviteURL.value || t("home.values.notConfigured")}
        source={settings.channelInviteURL.source}
      />
      <ConfigValue
        labelKey="home.entries.bypass.failOpen"
        value={t(settings.requiredChannelFailOpen.value ? "home.values.enabled" : "home.values.disabled")}
        source={settings.requiredChannelFailOpen.source}
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
      <ConfigValue
        labelKey="home.entries.moderation.adminLog"
        value={
          settings.adminLogChatID.value === 0
            ? t("home.values.disabled")
            : t("home.values.configured")
        }
        source={settings.adminLogChatID.source}
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
      styles={style({ display: "grid", gap: 16, minWidth: 0 })}
    >
      <Divider size="S" />
      <Content data-home-section-heading styles={style({ display: "grid", gap: 8 })}>
        <Heading level={2} id="home-entries-title" styles={style({ font: "heading", margin: 0 })}>
          {t("home.entries.title")}
        </Heading>
        <Text styles={style({ font: "body", color: "neutral-subdued" })}>
          {t("home.entries.description")}
        </Text>
      </Content>
      <Content
        data-home-entries
        styles={style({ display: "grid", gridTemplateColumns: ["minmax(0, 1fr)"], gap: 16, minWidth: 0 })}
      >
        <VerificationEntry settings={settings} groupSearch={groupSearch} />
        <QuestionsEntry settings={settings} groupSearch={groupSearch} />
        <BypassEntry settings={settings} groupSearch={groupSearch} />
        <ModerationEntry settings={settings} groupSearch={groupSearch} />
      </Content>
    </Content>
  );
}
