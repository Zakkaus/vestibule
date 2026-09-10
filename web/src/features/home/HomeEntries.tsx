import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Content, Heading, Link, Text } from "@react-spectrum/s2";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };

import { sectionSurface } from "./surface";
import { Icon, type IconName } from "../../icons";
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
  iconName,
  path,
  groupSearch,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  iconName: IconName;
  path: string;
  groupSearch: string;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    // One row: a grid of [label over value] and the arrow. Link takes only positioning
    // styles, so the surface sits on the one element inside it.
    <Link href={`${path}${groupSearch}`} isStandalone isQuiet data-home-entry={id}>
      <Content styles={style({
        display: "grid",
        gridTemplateColumns: ["minmax(0, 1fr)", "auto"],
        alignItems: "center",
        columnGap: 12,
        rowGap: 4,
        minWidth: 0,
        padding: 12,
        borderRadius: "lg",
        backgroundColor: "layer-1"
      })}>
        <Text styles={style({ display: "flex", alignItems: "center", gap: 4, font: "ui-sm", fontWeight: "medium", color: "neutral-subdued" })}>
          <Icon name={iconName} /> {t(titleKey)}
        </Text>
        {/* The arrow is an affordance, not an action: it takes the label's colour so the
            accent stays reserved for things you press. */}
        <Content styles={style({ gridRowStart: 1, gridRowEnd: 3, gridColumnStart: 2, display: "flex", alignItems: "center", color: "neutral-subdued" })}>
          <Icon name="arrowRight" />
        </Content>
        <Content data-home-entry-values styles={style({ minWidth: 0, gridColumnStart: 1 })}>{children}</Content>
      </Content>
    </Link>
  );
}

type EntryProps = Readonly<{ settings: HomeSettings; groupSearch: string }>;

function VerificationEntry({ settings, groupSearch }: EntryProps) {
  const { t } = useTranslation();
  return (
    <ConfigEntry
      id="verification"
      titleKey="home.entries.verification.title"
      iconName="shieldCheck"
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
      iconName="bookOpen"
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
      iconName="usersRound"
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
      iconName="shieldAlert"
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
      styles={sectionSurface}
    >
      <Content data-home-section-heading>
        <Heading level={2} id="home-entries-title" styles={style({ font: "heading-lg", margin: 0 })}>
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
