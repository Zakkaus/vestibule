import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import { Icon } from "../../icons";
import { StatusBadge, type StatusTone } from "../../components/StatusBadge";
import { useConsoleSession } from "../../app/session";
import type { SettingSource } from "../verification/api";
import type { HomeData, HomeDataState } from "./useHomeData";
import { useHomeData } from "./useHomeData";
import { HomeTrend } from "./HomeTrend";

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

type AttentionItem = Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  tone: StatusTone;
  count?: number;
}>;


function SourceBadge({ source }: Readonly<{ source: SettingSource }>) {
  const { t } = useTranslation();
  return (
    <StatusBadge tone="neutral">
      {t("home.source.value", { source: t(sourceMessageKeys[source]) })}
    </StatusBadge>
  );
}

function StateCard({
  id,
  titleKey,
  descriptionKey,
  role,
  live,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  role?: "alert";
  live?: "polite";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-home-state-card={id}
      role={role}
      aria-live={live}
      aria-labelledby={`home-${id}-title`}
    >
      <h2 id={`home-${id}-title`}>{t(titleKey)}</h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
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
    <div data-home-entry-value>
      <dt>{t(labelKey)}</dt>
      <dd>
        <span>{value}</span>
        <SourceBadge source={source} />
      </dd>
    </div>
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
    <Link
      to={{ pathname: path, search: groupSearch }}
      data-slot="card"
      data-clickable
      data-home-entry={id}
    >
      <header data-home-entry-heading>
        <h3>{t(titleKey)}</h3>
        <span aria-hidden="true">→</span>
      </header>
      <p>{t(descriptionKey)}</p>
      <dl data-home-entry-values>{children}</dl>
    </Link>
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
        <h2 id="home-overview-title">{t("home.overview.title")}</h2>
        <p>{t("home.overview.description")}</p>
      </header>
      <div data-home-metrics>
        {metrics.map((metric) => (
          <Link
            key={metric.id}
            to={{ pathname: metric.path, search: groupSearch }}
            data-slot="card"
            data-clickable
            data-home-metric={metric.id}
          >
            <strong>{metric.value}</strong>
            <span>{t(metric.labelKey)}</span>
          </Link>
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
        <h2 id="home-attention-title">{t("home.attention.title")}</h2>
        <p>{t("home.attention.description")}</p>
      </header>
      {items.length === 0 ? (
        <div data-slot="card" data-home-attention-empty>
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
            <Link
              key={item.id}
              to={{
                pathname: item.id === "queue" ? "/queue" : "/diagnostics",
                search: groupSearch
              }}
              data-slot="card"
              data-clickable
              data-home-attention={item.id}
            >
              <StatusBadge tone={item.tone}>{t(`home.attention.tones.${item.tone}`)}</StatusBadge>
              <span data-home-attention-copy>
                <strong>{t(item.titleKey)}</strong>
                <span>{t(item.descriptionKey, { count: item.count })}</span>
              </span>
              <span aria-hidden="true">→</span>
            </Link>
          ))}
        </div>
      )}
    </section>
  );
}


function EntriesSection({
  data,
  groupSearch
}: Readonly<{ data: HomeData; groupSearch: string }>) {
  const { t } = useTranslation();
  const { settings } = data;

  return (
    <section data-home-section="entries" aria-labelledby="home-entries-title">
      <header data-home-section-heading>
        <h2 id="home-entries-title">{t("home.entries.title")}</h2>
        <p>{t("home.entries.description")}</p>
      </header>
      <div data-home-entries>
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
            value={t(settings.antispamEnabled.value ? "home.values.enabled" : "home.values.disabled")}
            source={settings.antispamEnabled.source}
          />
          <ConfigValue
            labelKey="home.entries.moderation.warnLimit"
            value={t("home.values.warnings", { count: settings.warnLimit.value })}
            source={settings.warnLimit.source}
          />
        </ConfigEntry>
      </div>
    </section>
  );
}

function LoadedHome({ data, chatID }: Readonly<{ data: HomeData; chatID: string }>) {
  const { t } = useTranslation();
  const groupSearch = `?${new URLSearchParams({ group: chatID }).toString()}`;
  const isOperator = data.diagnostics.kind !== "hidden";

  return (
    <div data-home-content>
      <aside data-slot="card" data-home-context aria-labelledby="home-context-title">
        <span data-home-context-copy>
          <strong id="home-context-title">{t("home.context.title", { id: chatID })}</strong>
          <span>{t("home.context.selectedScope")}</span>
        </span>
        <span data-home-context-badges>
          <StatusBadge tone="neutral">
            {t(isOperator ? "home.roles.operator" : "home.roles.manager")}
          </StatusBadge>
          <StatusBadge tone={data.settings.enabled.value ? "ok" : "neutral"}>
            {t(data.settings.enabled.value ? "home.context.enabled" : "home.context.disabled")}
          </StatusBadge>
          <SourceBadge source={data.settings.enabled.source} />
          <StatusBadge tone="neutral">{t("home.context.modeUnreported")}</StatusBadge>
        </span>
      </aside>
      <OverviewSection data={data} groupSearch={groupSearch} />
      <AttentionSection data={data} groupSearch={groupSearch} />
      <HomeTrend data={data} groupSearch={groupSearch} />
      <EntriesSection data={data} groupSearch={groupSearch} />
    </div>
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
        titleKey="home.groupRequired.title"
        descriptionKey="home.groupRequired.description"
      >
        <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
          <Icon name="usersRound" />
          {t("home.groupRequired.select")}
        </Link>
      </StateCard>
    );
  }
  if (state.kind === "no-groups") {
    return (
      <StateCard
        id="no-groups"
        titleKey="home.noGroups.title"
        descriptionKey="home.noGroups.description"
      />
    );
  }
  return (
    <StateCard
      id="unavailable"
      titleKey="home.unavailable.title"
      descriptionKey="home.unavailable.description"
      role="alert"
    >
      <button
        type="button"
        data-slot="button"
        data-variant="outline"
        data-size="sm"
        onClick={reload}
      >
        <Icon name="refreshCw" />
        {t("home.unavailable.retry")}
      </button>
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
    <section
      data-home-page
      data-home-state={controller.state.kind}
      aria-busy={controller.state.kind === "loading" || undefined}
      aria-labelledby="home-title"
    >
      <header data-page-heading>
        <h1 id="home-title">{t("home.title")}</h1>
        <p>{t("home.description")}</p>
      </header>
      <HomeStateContent state={controller.state} chatID={chatID} reload={controller.reload} />
    </section>
  );
}
