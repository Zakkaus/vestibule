import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export const processSettingSources = ["factory default", "user file"] as const;
export type ProcessSettingSource = (typeof processSettingSources)[number];

export const feedLanguages = ["", "zh", "zh-Hant", "en"] as const;
export type FeedLanguage = (typeof feedLanguages)[number];

export type ProcessSetting<T> = Readonly<{
  value: T;
  source: ProcessSettingSource;
}>;

export type FeedConfig = Readonly<{
  chatID: number;
  lang: FeedLanguage;
  intervalSeconds: number;
  bugs: boolean | null;
  news: boolean | null;
  bugProduct: string;
  bugComponent: string;
  silentBugs: boolean | null;
}>;

export type OverlayConfig = Readonly<{
  name: string;
  repo: string;
  branch: string;
}>;

export type FeedSettings = Readonly<{
  feeds: ProcessSetting<readonly FeedConfig[]>;
  newsURL: ProcessSetting<string>;
  overlays: ProcessSetting<readonly OverlayConfig[]>;
}>;

type PayloadParser<T> = (value: unknown) => T | undefined;

function stringFromPayload(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}


function nullableBooleanFromPayload(value: unknown): boolean | null | undefined {
  return value === null || typeof value === "boolean" ? value : undefined;
}


function arrayFromPayload<T>(value: unknown, parse: PayloadParser<T>): readonly T[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  const parsed: T[] = [];
  for (const item of value) {
    const next = parse(item);
    if (next === undefined) {
      return undefined;
    }
    parsed.push(next);
  }
  return parsed;
}

function settingFromPayload<T>(
  payload: unknown,
  parseValue: PayloadParser<T>
): ProcessSetting<T> | undefined {
  const field = objectFromPayload(payload);
  if (!field) {
    return undefined;
  }

  const value = parseValue(field.value);
  const source = field.source;
  if (
    value === undefined ||
    typeof source !== "string" ||
    !processSettingSources.includes(source as ProcessSettingSource)
  ) {
    return undefined;
  }
  return { value, source: source as ProcessSettingSource };
}


function feedFromPayload(payload: unknown): FeedConfig | undefined {
  const feed = objectFromPayload(payload);
  if (!feed) {
    return undefined;
  }

  const chatID =
    typeof feed.chat_id === "number" && Number.isSafeInteger(feed.chat_id)
      ? feed.chat_id
      : undefined;
  const lang =
    typeof feed.lang === "string" && feedLanguages.includes(feed.lang as FeedLanguage)
      ? (feed.lang as FeedLanguage)
      : undefined;
  const intervalSeconds =
    typeof feed.interval_seconds === "number" && Number.isSafeInteger(feed.interval_seconds)
      ? feed.interval_seconds
      : undefined;
  const bugs = nullableBooleanFromPayload(feed.bugs);
  const news = nullableBooleanFromPayload(feed.news);
  const bugProduct = stringFromPayload(feed.bug_product);
  const bugComponent = stringFromPayload(feed.bug_component);
  const silentBugs = nullableBooleanFromPayload(feed.silent_bugs);
  if (
    chatID === undefined ||
    lang === undefined ||
    intervalSeconds === undefined ||
    bugs === undefined ||
    news === undefined ||
    bugProduct === undefined ||
    bugComponent === undefined ||
    silentBugs === undefined
  ) {
    return undefined;
  }

  return {
    chatID,
    lang,
    intervalSeconds,
    bugs,
    news,
    bugProduct,
    bugComponent,
    silentBugs
  };
}

function overlayFromPayload(payload: unknown): OverlayConfig | undefined {
  const overlay = objectFromPayload(payload);
  if (!overlay) {
    return undefined;
  }

  const name = stringFromPayload(overlay.name);
  const repo = stringFromPayload(overlay.repo);
  const branch = stringFromPayload(overlay.branch);
  return name === undefined || repo === undefined || branch === undefined
    ? undefined
    : { name, repo, branch };
}

function feedSettingsFromPayload(payload: unknown): FeedSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const feeds = settingFromPayload(response.feeds, (value) => arrayFromPayload(value, feedFromPayload));
  const newsURL = settingFromPayload(response.news_url, stringFromPayload);
  const overlays = settingFromPayload(response.overlays, (value) =>
    arrayFromPayload(value, overlayFromPayload)
  );
  return feeds && newsURL && overlays ? { feeds, newsURL, overlays } : undefined;
}

export function loadFeedSettings(transport: ApiTransport): Promise<ApiResult<FeedSettings>> {
  return transport.request("/api/process/settings", { parse: feedSettingsFromPayload });
}
