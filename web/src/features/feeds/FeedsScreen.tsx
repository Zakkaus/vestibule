import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { consoleApi, useConsoleSession } from "../../app/session";
import { StatusBadge } from "../../components/StatusBadge";
import type { ApiRequestError } from "../../lib/api";
import {
  loadFeedSettings,
  type FeedConfig,
  type FeedLanguage,
  type FeedSettings,
  type OverlayConfig,
  type ProcessSettingSource
} from "./api";

type FeedsScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; settings: FeedSettings }>
  | Readonly<{ kind: "access-denied" }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

const sourceMessageKeys: Readonly<Record<ProcessSettingSource, string>> = {
  "factory default": "feeds.source.factoryDefault",
  "user file": "feeds.source.userFile"
};

const languageMessageKeys: Readonly<Record<FeedLanguage, string>> = {
  "": "feeds.languages.default",
  zh: "feeds.languages.zh",
  "zh-Hant": "feeds.languages.zhHant",
  en: "feeds.languages.en"
};

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "feeds.errors.authenticationExpired",
  authentication_invalid: "feeds.errors.authenticationInvalid",
  process_settings_unavailable: "feeds.errors.settingsUnavailable"
};

function processSettingsErrorMessageKey(error: ApiRequestError): string {
  if (error.kind === "network") {
    return "feeds.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? "feeds.errors.loadUnavailable";
  }
  return "feeds.errors.invalidResponse";
}

function ProcessSourceBadge({ source }: Readonly<{ source: ProcessSettingSource }>) {
  const { t } = useTranslation();
  return (
    <StatusBadge tone="neutral">
      {t("feeds.source.value", { source: t(sourceMessageKeys[source]) })}
    </StatusBadge>
  );
}

function BooleanValue({
  value,
  defaultValue
}: Readonly<{ value: boolean | null; defaultValue: boolean }>) {
  const { t } = useTranslation();
  const effectiveValue = value ?? defaultValue;
  const messageKey =
    value === null
      ? effectiveValue
        ? "feeds.values.enabledByDefault"
        : "feeds.values.disabledByDefault"
      : effectiveValue
        ? "feeds.values.enabled"
        : "feeds.values.disabled";
  return <StatusBadge tone={effectiveValue ? "ok" : "neutral"}>{t(messageKey)}</StatusBadge>;
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
      data-feeds-state-card={id}
      role={role}
      aria-live={live}
      aria-labelledby={`feeds-${id}-title`}
    >
      <h2 id={`feeds-${id}-title`}>{t(titleKey)}</h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

function SectionHeading({
  id,
  titleKey,
  descriptionKey,
  source
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  source: ProcessSettingSource;
}>) {
  const { t } = useTranslation();
  return (
    <header data-feeds-section-heading>
      <div data-feeds-section-copy>
        <h2 id={id}>{t(titleKey)}</h2>
        <p>{t(descriptionKey)}</p>
      </div>
      <ProcessSourceBadge source={source} />
    </header>
  );
}

function FeedItem({ feed, number }: Readonly<{ feed: FeedConfig; number: number }>) {
  const { t } = useTranslation();
  return (
    <article className="surface-raised" data-feed-item aria-labelledby={`feed-item-${number}-title`}>
      <h3 id={`feed-item-${number}-title`}>{t("feeds.feed.itemTitle", { number })}</h3>
      <dl data-feed-values>
        <div data-feed-value>
          <dt>{t("feeds.feed.destination")}</dt>
          <dd><code>{feed.chatID}</code></dd>
        </div>
        <div data-feed-value>
          <dt>{t("feeds.feed.language")}</dt>
          <dd>{t(languageMessageKeys[feed.lang])}</dd>
        </div>
        <div data-feed-value>
          <dt>{t("feeds.feed.interval")}</dt>
          <dd>{t("feeds.values.seconds", { count: feed.intervalSeconds })}</dd>
        </div>
        <div data-feed-value>
          <dt>{t("feeds.feed.bugzilla")}</dt>
          <dd><BooleanValue value={feed.bugs} defaultValue /></dd>
        </div>
        <div data-feed-value>
          <dt>{t("feeds.feed.news")}</dt>
          <dd><BooleanValue value={feed.news} defaultValue /></dd>
        </div>
        <div data-feed-value>
          <dt>{t("feeds.feed.bugProduct")}</dt>
          <dd>{feed.bugProduct || t("feeds.feed.allProducts")}</dd>
        </div>
        <div data-feed-value>
          <dt>{t("feeds.feed.bugComponent")}</dt>
          <dd>{feed.bugComponent || t("feeds.feed.allComponents")}</dd>
        </div>
        <div data-feed-value>
          <dt>{t("feeds.feed.silentBugs")}</dt>
          <dd><BooleanValue value={feed.silentBugs} defaultValue={false} /></dd>
        </div>
      </dl>
    </article>
  );
}

function FeedListSection({ settings }: Readonly<{ settings: FeedSettings["feeds"] }>) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-feeds-section="feeds"
      data-process-setting-source={settings.source}
      aria-labelledby="feeds-destinations-title"
    >
      <SectionHeading
        id="feeds-destinations-title"
        titleKey="feeds.feed.title"
        descriptionKey="feeds.feed.description"
        source={settings.source}
      />
      {settings.value.length === 0 ? (
        <div className="surface-raised" data-feeds-empty>
          <strong>{t("feeds.feed.emptyTitle")}</strong>
          <p>{t("feeds.feed.emptyDescription")}</p>
        </div>
      ) : (
        <div data-feed-list>
          {settings.value.map((feed, index) => (
            <FeedItem key={`${feed.chatID}:${index}`} feed={feed} number={index + 1} />
          ))}
        </div>
      )}
    </section>
  );
}

function NewsURLSection({ settings }: Readonly<{ settings: FeedSettings["newsURL"] }>) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-feeds-section="news-url"
      data-process-setting-source={settings.source}
      aria-labelledby="feeds-news-url-title"
    >
      <SectionHeading
        id="feeds-news-url-title"
        titleKey="feeds.newsURL.title"
        descriptionKey="feeds.newsURL.description"
        source={settings.source}
      />
      <div className="surface-raised" data-news-url-value>
        {settings.value ? <code>{settings.value}</code> : <span>{t("feeds.newsURL.empty")}</span>}
      </div>
    </section>
  );
}

function OverlayItem({ overlay, number }: Readonly<{ overlay: OverlayConfig; number: number }>) {
  const { t } = useTranslation();
  return (
    <article
      className="surface-raised"
      data-overlay-item
      aria-labelledby={`overlay-item-${number}-title`}
    >
      <h3 id={`overlay-item-${number}-title`}>{overlay.name || overlay.repo}</h3>
      <dl data-overlay-values>
        <div data-overlay-value>
          <dt>{t("feeds.overlays.repository")}</dt>
          <dd><code>{overlay.repo}</code></dd>
        </div>
        <div data-overlay-value>
          <dt>{t("feeds.overlays.branch")}</dt>
          <dd><code>{overlay.branch || t("feeds.overlays.defaultBranch")}</code></dd>
        </div>
      </dl>
    </article>
  );
}

function OverlaySection({ settings }: Readonly<{ settings: FeedSettings["overlays"] }>) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-feeds-section="overlays"
      data-process-setting-source={settings.source}
      aria-labelledby="feeds-overlays-title"
    >
      <SectionHeading
        id="feeds-overlays-title"
        titleKey="feeds.overlays.title"
        descriptionKey="feeds.overlays.description"
        source={settings.source}
      />
      {settings.value.length === 0 ? (
        <div className="surface-raised" data-overlays-empty>
          <strong>{t("feeds.overlays.emptyTitle")}</strong>
          <p>{t("feeds.overlays.emptyDescription")}</p>
        </div>
      ) : (
        <div data-overlay-list>
          {settings.value.map((overlay, index) => (
            <OverlayItem key={`${overlay.name}:${overlay.repo}:${index}`} overlay={overlay} number={index + 1} />
          ))}
        </div>
      )}
    </section>
  );
}

function LoadedFeedSettings({ settings }: Readonly<{ settings: FeedSettings }>) {
  const { t } = useTranslation();
  return (
    <div data-feeds-content>
      <aside data-slot="card" data-feeds-readonly aria-labelledby="feeds-readonly-title">
        <header data-feeds-readonly-heading>
          <h2 id="feeds-readonly-title">{t("feeds.readOnly.title")}</h2>
          <StatusBadge tone="neutral">{t("feeds.readOnly.badge")}</StatusBadge>
        </header>
        <p>{t("feeds.readOnly.description")}</p>
      </aside>
      <FeedListSection settings={settings.feeds} />
      <NewsURLSection settings={settings.newsURL} />
      <OverlaySection settings={settings.overlays} />
    </div>
  );
}

export function FeedsScreen() {
  const { t } = useTranslation();
  const session = useConsoleSession();
  const [screenState, setScreenState] = useState<FeedsScreenState>({ kind: "loading" });
  const [reloadVersion, setReloadVersion] = useState(0);

  useEffect(() => {
    let active = true;
    if (session.state === "loading" || session.state === "checking-groups") {
      setScreenState({ kind: "loading" });
      return () => {
        active = false;
      };
    }
    if (session.state === "blocked") {
      setScreenState({ kind: "unavailable", error: session.error });
      return () => {
        active = false;
      };
    }

    setScreenState({ kind: "loading" });
    void loadFeedSettings(consoleApi).then((result) => {
      if (!active) {
        return;
      }
      if (result.ok) {
        setScreenState({ kind: "loaded", settings: result.data });
        return;
      }
      if (result.error.kind === "api" && result.error.code === "process_access_denied") {
        setScreenState({ kind: "access-denied" });
        return;
      }
      setScreenState({ kind: "unavailable", error: result.error });
    });

    return () => {
      active = false;
    };
  }, [reloadVersion, session]);

  return (
    <section
      data-feeds-page
      data-feeds-state={screenState.kind}
      aria-busy={screenState.kind === "loading" || undefined}
      aria-labelledby="feeds-title"
    >
      <header data-page-heading>
        <h1 id="feeds-title">{t("feeds.title")}</h1>
        <p>{t("feeds.description")}</p>
      </header>
      {screenState.kind === "loading" ? (
        <StateCard
          id="loading"
          titleKey="feeds.loading.title"
          descriptionKey="feeds.loading.description"
          live="polite"
        />
      ) : null}
      {screenState.kind === "access-denied" ? (
        <StateCard
          id="access-denied"
          titleKey="feeds.accessDenied.title"
          descriptionKey="feeds.accessDenied.description"
          role="alert"
        />
      ) : null}
      {screenState.kind === "unavailable" ? (
        <StateCard
          id="unavailable"
          titleKey="feeds.unavailable.title"
          descriptionKey={processSettingsErrorMessageKey(screenState.error)}
          role="alert"
        >
          <button
            type="button"
            data-slot="button"
            data-variant="outline"
            data-size="sm"
            onClick={() => setReloadVersion((version) => version + 1)}
          >
            {t("feeds.unavailable.retry")}
          </button>
        </StateCard>
      ) : null}
      {screenState.kind === "loaded" ? <LoadedFeedSettings settings={screenState.settings} /> : null}
    </section>
  );
}
