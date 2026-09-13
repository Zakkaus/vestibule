import type { FeedLanguage, FeedSettings, FeedWriteValues } from "./api";

export type FeedRepositoryDraft = Readonly<{
  repo: string;
  branch: string;
  issues: boolean;
  pulls: boolean;
}>;

export type FeedDraft = Readonly<{
  lang: FeedLanguage;
  interval_seconds: string;
  bugs: boolean;
  news: boolean;
  bug_product: string;
  bug_component: string;
  silent_bugs: boolean;
  github_repos: readonly FeedRepositoryDraft[];
}>;

export type FeedField =
  | "lang"
  | "interval_seconds"
  | "bugs"
  | "news"
  | "bug_product"
  | "bug_component"
  | "silent_bugs";

export type FeedFieldError = Readonly<Partial<Record<string, string>>>;

export type FeedValidation = Readonly<{
  errors: FeedFieldError;
  values?: FeedWriteValues;
}>;

export const FEED_MIN_INTERVAL_SECONDS = 60;
export const FEED_MAX_INTERVAL_SECONDS = 86400;

const factoryValues: FeedWriteValues = {
  lang: "",
  interval_seconds: 300,
  bugs: false,
  news: false,
  bug_product: "",
  bug_component: "",
  silent_bugs: false,
  github_repos: []
};

export function settingsDraft(settings: FeedSettings): FeedDraft {
  return {
    lang: settings.feed.lang.value,
    interval_seconds: String(settings.feed.intervalSeconds.value),
    bugs: settings.feed.bugs.value,
    news: settings.feed.news.value,
    bug_product: settings.feed.bugProduct.value,
    bug_component: settings.feed.bugComponent.value,
    silent_bugs: settings.feed.silentBugs.value,
    github_repos: settings.githubRepos.value.map((repository) => ({ ...repository }))
  };
}

export function factoryDraft(): FeedDraft {
  return {
    lang: factoryValues.lang,
    interval_seconds: String(factoryValues.interval_seconds),
    bugs: factoryValues.bugs,
    news: factoryValues.news,
    bug_product: factoryValues.bug_product,
    bug_component: factoryValues.bug_component,
    silent_bugs: factoryValues.silent_bugs,
    github_repos: []
  };
}

function integerFromDraft(value: string): number | undefined {
  if (!/^-?\d+$/.test(value)) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : undefined;
}

export function validateDraft(draft: FeedDraft): FeedValidation {
  const intervalSeconds = integerFromDraft(draft.interval_seconds);
  const errors: Record<string, string> = {};
  if (intervalSeconds === undefined || intervalSeconds < FEED_MIN_INTERVAL_SECONDS || intervalSeconds > FEED_MAX_INTERVAL_SECONDS) {
    errors.interval_seconds = "feeds.validation.invalidInterval";
  }
  if (Object.keys(errors).length > 0 || intervalSeconds === undefined) {
    return { errors };
  }
  return {
    errors,
    values: {
      lang: draft.lang,
      interval_seconds: intervalSeconds,
      bugs: draft.bugs,
      news: draft.news,
      bug_product: draft.bug_product,
      bug_component: draft.bug_component,
      silent_bugs: draft.silent_bugs,
      github_repos: draft.github_repos.map((repository) => ({ ...repository }))
    }
  };
}

export function hasDraftChanges(settings: FeedSettings, draft: FeedDraft): boolean {
  const current = settingsDraft(settings);
  return (
    draft.lang !== current.lang ||
    draft.interval_seconds !== current.interval_seconds ||
    draft.bugs !== current.bugs ||
    draft.news !== current.news ||
    draft.bug_product !== current.bug_product ||
    draft.bug_component !== current.bug_component ||
    draft.silent_bugs !== current.silent_bugs ||
    JSON.stringify(draft.github_repos) !== JSON.stringify(current.github_repos)
  );
}
