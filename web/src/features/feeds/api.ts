import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export const feedSettingSources = ["factory default", "user file", "chat override"] as const;
export type FeedSettingSource = (typeof feedSettingSources)[number];
export const processSettingSources = ["factory default", "user file"] as const;
export type ProcessSettingSource = (typeof processSettingSources)[number];

export const feedLanguages = ["", "zh", "zh-Hant", "en", "ja", "ru"] as const;
export type FeedLanguage = (typeof feedLanguages)[number];

export type Setting<T, Source extends string = FeedSettingSource> = Readonly<{
  value: T;
  source: Source;
}>;

export type GitHubRepo = Readonly<{
  repo: string;
  branch: string;
  issues: boolean;
  pulls: boolean;
}>;

export type FeedWriteValues = Readonly<{
  lang: FeedLanguage;
  interval_seconds: number;
  bugs: boolean;
  news: boolean;
  bug_product: string;
  bug_component: string;
  silent_bugs: boolean;
  github_repos: readonly GitHubRepo[];
}>;

export type FeedView = Readonly<{
  lang: Setting<FeedLanguage>;
  intervalSeconds: Setting<number>;
  bugs: Setting<boolean>;
  news: Setting<boolean>;
  bugProduct: Setting<string>;
  bugComponent: Setting<string>;
  silentBugs: Setting<boolean>;
}>;

export type FeedSettings = Readonly<{
  revision: number;
  feed: FeedView;
  githubRepos: Setting<readonly GitHubRepo[]>;
}>;

export type OverlayConfig = Readonly<{
  name: string;
  repo: string;
  branch: string;
}>;

export type ProcessSettings = Readonly<{
  newsURL: Setting<string, ProcessSettingSource>;
  overlays: Setting<readonly OverlayConfig[], ProcessSettingSource>;
}>;

type PayloadParser<T> = (value: unknown) => T | undefined;

function object(value: unknown): Record<string, unknown> | undefined {
  return objectFromPayload(value) ?? undefined;
}

function stringFromPayload(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function integerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : undefined;
}

function booleanFromPayload(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function arrayFromPayload<T>(value: unknown, parse: PayloadParser<T>): readonly T[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const parsed: T[] = [];
  for (const item of value) {
    const next = parse(item);
    if (next === undefined) return undefined;
    parsed.push(next);
  }
  return parsed;
}

function settingFromPayload<T, Source extends string>(
  payload: unknown,
  parseValue: PayloadParser<T>,
  sources: readonly Source[]
): Setting<T, Source> | undefined {
  const field = object(payload);
  if (!field || typeof field.source !== "string" || !sources.includes(field.source as Source)) {
    return undefined;
  }
  const value = parseValue(field.value);
  return value === undefined ? undefined : { value, source: field.source as Source };
}

function githubRepoFromPayload(payload: unknown): GitHubRepo | undefined {
  const repo = object(payload);
  if (!repo) return undefined;
  const name = stringFromPayload(repo.repo);
  const branch = repo.branch === undefined ? "" : stringFromPayload(repo.branch);
  const issues = booleanFromPayload(repo.issues);
  const pulls = booleanFromPayload(repo.pulls);
  return name === undefined || branch === undefined || issues === undefined || pulls === undefined
    ? undefined
    : { repo: name, branch, issues, pulls };
}

function feedFromPayload(payload: unknown): FeedView | undefined {
  const feed = object(payload);
  if (!feed) return undefined;
  const lang = settingFromPayload(feed.lang, (value) =>
    typeof value === "string" && feedLanguages.includes(value as FeedLanguage) ? (value as FeedLanguage) : undefined,
    feedSettingSources
  );
  const intervalSeconds = settingFromPayload(feed.interval_seconds, integerFromPayload, feedSettingSources);
  const bugs = settingFromPayload(feed.bugs, booleanFromPayload, feedSettingSources);
  const news = settingFromPayload(feed.news, booleanFromPayload, feedSettingSources);
  const bugProduct = settingFromPayload(feed.bug_product, stringFromPayload, feedSettingSources);
  const bugComponent = settingFromPayload(feed.bug_component, stringFromPayload, feedSettingSources);
  const silentBugs = settingFromPayload(feed.silent_bugs, booleanFromPayload, feedSettingSources);
  return lang && intervalSeconds && bugs && news && bugProduct && bugComponent && silentBugs
    ? { lang, intervalSeconds, bugs, news, bugProduct, bugComponent, silentBugs }
    : undefined;
}

function feedSettingsFromPayload(payload: unknown): FeedSettings | undefined {
  const response = object(payload);
  if (!response) return undefined;
  const revision = integerFromPayload(response.revision);
  const feed = feedFromPayload(response.feed);
  const githubRepos = settingFromPayload(
    response.github_repos,
    (value) => arrayFromPayload(value, githubRepoFromPayload),
    feedSettingSources
  );
  return revision === undefined || revision < 0 || !feed || !githubRepos
    ? undefined
    : { revision, feed, githubRepos };
}

function overlayFromPayload(payload: unknown): OverlayConfig | undefined {
  const overlay = object(payload);
  if (!overlay) return undefined;
  const name = stringFromPayload(overlay.name);
  const repo = stringFromPayload(overlay.repo);
  const branch = stringFromPayload(overlay.branch);
  return name === undefined || repo === undefined || branch === undefined ? undefined : { name, repo, branch };
}

function processSettingsFromPayload(payload: unknown): ProcessSettings | undefined {
  const response = object(payload);
  if (!response) return undefined;
  const newsURL = settingFromPayload(response.news_url, stringFromPayload, processSettingSources);
  const overlays = settingFromPayload(
    response.overlays,
    (value) => arrayFromPayload(value, overlayFromPayload),
    processSettingSources
  );
  return newsURL && overlays ? { newsURL, overlays } : undefined;
}

export function loadFeedSettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<FeedSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/feeds`, { parse: feedSettingsFromPayload });
}

export function saveFeedSettings(
  transport: ApiTransport,
  chatID: string,
  expectedRevision: number,
  values: FeedWriteValues
): Promise<ApiResult<FeedSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/feeds`, {
    method: "PUT",
    body: { expected_revision: expectedRevision, ...values },
    parse: feedSettingsFromPayload
  });
}

export function loadProcessSettings(transport: ApiTransport): Promise<ApiResult<ProcessSettings>> {
  return transport.request("/api/process/settings", { parse: processSettingsFromPayload });
}
