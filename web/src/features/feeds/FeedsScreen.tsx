import { Fragment, useEffect, useState, type ReactNode } from "react";
import { Button, Text } from "@react-spectrum/s2/Button";
import { useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { consoleApi, retryConsoleAccess, useConsoleSession } from "../../app/session";
import { StatusBadge } from "../../components/StatusBadge";
import { Icon } from "../../icons";
import type { IconName } from "../../icons";
import { ApiError, type ApiRequestError } from "../../lib/api";
import { groupName } from "../../lib/chatNames";
import {
  loadFeedSettings,
  loadProcessSettings,
  type FeedLanguage,
  type FeedSettings,
  type FeedView,
  type GitHubRepo,
  type OverlayConfig,
  type ProcessSettings,
  type FeedSettingSource,
  type ProcessSettingSource,
  type Setting
} from "./api";

type FeedsScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; chatID: string; settings: FeedSettings; process?: ProcessSettings }>
  | Readonly<{ kind: "access-denied" }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

const languageMessageKeys: Readonly<Record<FeedLanguage, string>> = {
  "": "feeds.languages.default", zh: "feeds.languages.zh", "zh-Hant": "feeds.languages.zhHant",
  en: "feeds.languages.en", ja: "feeds.languages.ja", ru: "feeds.languages.ru"
};
const sourceMessageKeys: Readonly<Record<FeedSettingSource | ProcessSettingSource, string>> = {
  "factory default": "feeds.source.factoryDefault", "user file": "feeds.source.userFile", "chat override": "feeds.source.chatOverride"
};
const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "feeds.errors.authenticationExpired",
  authentication_invalid: "feeds.errors.authenticationInvalid",
  chat_not_found: "feeds.errors.loadUnavailable",
  settings_unavailable: "feeds.errors.settingsUnavailable",
  process_settings_unavailable: "feeds.errors.settingsUnavailable"
};

function errorMessageKey(error: ApiRequestError): string {
  if (error.kind === "network") return "feeds.errors.network";
  if (error.kind === "api") return errorMessageKeys[error.code] ?? "feeds.errors.loadUnavailable";
  return "feeds.errors.invalidResponse";
}

function SourceBadge({ source }: Readonly<{ source: FeedSettingSource | ProcessSettingSource }>) {
  const { t } = useTranslation();
  return <StatusBadge tone="neutral">{t("feeds.source.value", { source: t(sourceMessageKeys[source]) })}</StatusBadge>;
}

function BooleanValue({ value }: Readonly<{ value: boolean }>) {
  const { t } = useTranslation();
  return <StatusBadge tone={value ? "ok" : "neutral"}>{t(value ? "feeds.values.enabled" : "feeds.values.disabled")}</StatusBadge>;
}

function StateCard({ id, titleKey, descriptionKey, iconName, role, live, children }: Readonly<{
  id: string; titleKey: string; descriptionKey: string; iconName: IconName; role?: "alert"; live?: "polite"; children?: ReactNode;
}>) {
  const { t } = useTranslation();
  return <section data-slot="card" data-feeds-state-card={id} role={role} aria-live={live} aria-labelledby={`feeds-${id}-title`}>
    <h2 id={`feeds-${id}-title`}><span data-state-heading><Icon name={iconName} />{t(titleKey)}</span></h2>
    <p>{t(descriptionKey)}</p>{children}
  </section>;
}

function SectionHeading({ id, titleKey, descriptionKey, source }: Readonly<{
  id: string; titleKey: string; descriptionKey: string; source: FeedSettingSource | ProcessSettingSource;
}>) {
  const { t } = useTranslation();
  return <header data-feeds-section-heading><div data-feeds-section-copy><h2 id={id}>{t(titleKey)}</h2><p>{t(descriptionKey)}</p></div><SourceBadge source={source} /></header>;
}

function FeedValue<T>({ setting, children }: Readonly<{ setting: Setting<T>; children: (value: T) => ReactNode }>) {
  return <><dd>{children(setting.value)}</dd><small><SourceBadge source={setting.source} /></small></>;
}

function FeedSection({ feed, githubRepos, chatID, title }: Readonly<{ feed: FeedView; githubRepos: Setting<readonly GitHubRepo[]>; chatID: string; title?: string }>) {
  const { t } = useTranslation();
  return <section data-slot="card" data-feeds-section="feeds" data-feed-setting-source={githubRepos.source} aria-labelledby="feeds-destinations-title">
    <SectionHeading id="feeds-destinations-title" titleKey="feeds.feed.title" descriptionKey="feeds.feed.description" source={githubRepos.source} />
    <article className="surface-raised" data-feed-item aria-labelledby="feed-item-title">
      <h3 id="feed-item-title">{title ?? groupName(chatID)}</h3>
      <dl data-feed-values>
        <div data-feed-value><dt>{t("feeds.feed.destination")}</dt><dd><code>{groupName(chatID, title)}</code></dd></div>
        <div data-feed-value><dt>{t("feeds.feed.language")}</dt><FeedValue setting={feed.lang}>{(value) => t(languageMessageKeys[value])}</FeedValue></div>
        <div data-feed-value><dt>{t("feeds.feed.interval")}</dt><FeedValue setting={feed.intervalSeconds}>{(value) => t("feeds.values.seconds", { count: value })}</FeedValue></div>
        <div data-feed-value><dt>{t("feeds.feed.bugzilla")}</dt><FeedValue setting={feed.bugs}>{(value) => <BooleanValue value={value} />}</FeedValue></div>
        <div data-feed-value><dt>{t("feeds.feed.news")}</dt><FeedValue setting={feed.news}>{(value) => <BooleanValue value={value} />}</FeedValue></div>
        <div data-feed-value><dt>{t("feeds.feed.bugProduct")}</dt><FeedValue setting={feed.bugProduct}>{(value) => value || t("feeds.feed.allProducts")}</FeedValue></div>
        <div data-feed-value><dt>{t("feeds.feed.bugComponent")}</dt><FeedValue setting={feed.bugComponent}>{(value) => value || t("feeds.feed.allComponents")}</FeedValue></div>
        <div data-feed-value><dt>{t("feeds.feed.silentBugs")}</dt><FeedValue setting={feed.silentBugs}>{(value) => <BooleanValue value={value} />}</FeedValue></div>
        {githubRepos.value.map((repo, index) => <GitHubRepoItem key={`${repo.repo}:${repo.branch}:${index}`} repo={repo} />)}
      </dl>
    </article>
  </section>;
}

function GitHubRepoItem({ repo }: Readonly<{ repo: GitHubRepo }>) {
  const { t } = useTranslation();
  return <Fragment><div data-feed-value><dt>{t("feeds.feed.githubRepository")}</dt><dd><code>{repo.repo}</code><span aria-hidden="true"> · </span>{repo.branch ? <code>{repo.branch}</code> : t("feeds.feed.followingDefault")}</dd></div>
    <div data-feed-value><dt>{t("feeds.feed.githubIssues")}</dt><dd><BooleanValue value={repo.issues} /></dd></div>
    <div data-feed-value><dt>{t("feeds.feed.githubPulls")}</dt><dd><BooleanValue value={repo.pulls} /></dd></div></Fragment>;
}

function NewsURLSection({ settings }: Readonly<{ settings: ProcessSettings["newsURL"] }>) {
  const { t } = useTranslation();
  return <section data-slot="card" data-feeds-section="news-url" data-process-setting-source={settings.source} aria-labelledby="feeds-news-url-title">
    <SectionHeading id="feeds-news-url-title" titleKey="feeds.newsURL.title" descriptionKey="feeds.newsURL.description" source={settings.source} />
    <div className="surface-raised" data-news-url-value>{settings.value ? <code>{settings.value}</code> : <span data-state-heading><Icon name="inbox" />{t("feeds.newsURL.empty")}</span>}</div>
  </section>;
}

function OverlayItem({ overlay, number }: Readonly<{ overlay: OverlayConfig; number: number }>) {
  const { t } = useTranslation();
  return <article className="surface-raised" data-overlay-item aria-labelledby={`overlay-item-${number}-title`}><h3 id={`overlay-item-${number}-title`}>{overlay.name || overlay.repo}</h3><dl data-overlay-values><div data-overlay-value><dt>{t("feeds.overlays.repository")}</dt><dd><code>{overlay.repo}</code></dd></div><div data-overlay-value><dt>{t("feeds.overlays.branch")}</dt><dd><code>{overlay.branch || t("feeds.overlays.defaultBranch")}</code></dd></div></dl></article>;
}

function OverlaySection({ settings }: Readonly<{ settings: ProcessSettings["overlays"] }>) {
  const { t } = useTranslation();
  return <section data-slot="card" data-feeds-section="overlays" data-process-setting-source={settings.source} aria-labelledby="feeds-overlays-title"><SectionHeading id="feeds-overlays-title" titleKey="feeds.overlays.title" descriptionKey="feeds.overlays.description" source={settings.source} />{settings.value.length === 0 ? <div className="surface-raised" data-overlays-empty><div data-state-heading><Icon name="inbox" /><strong>{t("feeds.overlays.emptyTitle")}</strong></div><p>{t("feeds.overlays.emptyDescription")}</p></div> : <div data-overlay-list>{settings.value.map((overlay, index) => <OverlayItem key={`${overlay.name}:${index}`} overlay={overlay} number={index + 1} />)}</div>}</section>;
}

function LoadedFeedSettings({ settings, process, chatID, title }: Readonly<{ settings: FeedSettings; process?: ProcessSettings; chatID: string; title?: string }>) {
  const { t } = useTranslation();
  return <div data-feeds-content><aside data-slot="card" data-feeds-readonly aria-labelledby="feeds-readonly-title"><header data-feeds-readonly-heading><h2 id="feeds-readonly-title">{t("feeds.readOnly.title")}</h2><StatusBadge tone="neutral">{t("feeds.readOnly.badge")}</StatusBadge></header><p>{t("feeds.readOnly.description")}</p></aside><FeedSection feed={settings.feed} githubRepos={settings.githubRepos} chatID={chatID} title={title} />{process ? <><NewsURLSection settings={process.newsURL} /><OverlaySection settings={process.overlays} /></> : null}</div>;
}

export function FeedsScreen() {
  const { t } = useTranslation();
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const [screenState, setScreenState] = useState<FeedsScreenState>({ kind: "loading" });
  const [reloadVersion, setReloadVersion] = useState(0);
  useEffect(() => {
    let active = true;
    if (session.state === "loading" || session.state === "checking-groups") { setScreenState({ kind: "loading" }); return () => { active = false; }; }
    if (session.state === "blocked") { setScreenState({ kind: "unavailable", error: session.error }); return () => { active = false; }; }
    const selected = searchParams.get("group");
    const chatID = selected && /^-?\d+$/.test(selected) ? selected : session.state === "ready" ? session.chats[0]?.id : undefined;
    if (!chatID) { setScreenState({ kind: "unavailable", error: new ApiError("chat_not_found", 404) }); return () => { active = false; }; }
    setScreenState({ kind: "loading" });
    void Promise.all([loadFeedSettings(consoleApi, chatID), loadProcessSettings(consoleApi)]).then(([feedResult, processResult]) => {
      if (!active) return;
      if (!feedResult.ok) { setScreenState(feedResult.error.kind === "api" && feedResult.error.code === "chat_access_denied" ? { kind: "access-denied" } : { kind: "unavailable", error: feedResult.error }); return; }
      setScreenState({ kind: "loaded", chatID, settings: feedResult.data, process: processResult.ok ? processResult.data : undefined });
    });
    return () => { active = false; };
  }, [reloadVersion, searchParams, session]);
  function reloadFeeds(): void { if (retryConsoleAccess(session)) return; setReloadVersion((version) => version + 1); }
  const title = session.state === "ready" && screenState.kind === "loaded"
    ? session.chats.find((chat) => chat.id === screenState.chatID)?.title
    : undefined;
  return <section data-feeds-page data-feeds-state={screenState.kind} aria-busy={screenState.kind === "loading" || undefined} aria-labelledby="feeds-title"><header data-page-heading><h1 id="feeds-title">{t("feeds.title")}</h1><p>{t("feeds.description")}</p></header>{screenState.kind === "loading" ? <StateCard id="loading" titleKey="feeds.loading.title" descriptionKey="feeds.loading.description" live="polite" iconName="loaderCircle" /> : null}{screenState.kind === "access-denied" ? <StateCard id="access-denied" titleKey="feeds.accessDenied.title" descriptionKey="feeds.accessDenied.description" role="alert" iconName="circleAlert" /> : null}{screenState.kind === "unavailable" ? <StateCard id="unavailable" titleKey="feeds.unavailable.title" descriptionKey={errorMessageKey(screenState.error)} role="alert" iconName="circleAlert"><Button type="button" variant="primary" data-slot="button" onPress={reloadFeeds}><Icon name="refreshCw" /><Text>{t("feeds.unavailable.retry")}</Text></Button></StateCard> : null}{screenState.kind === "loaded" ? <LoadedFeedSettings settings={screenState.settings} process={screenState.process} chatID={screenState.chatID} title={title} /> : null}</section>;
}
