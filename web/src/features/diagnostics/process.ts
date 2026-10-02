import { objectFromPayload, type ApiResult, type ApiTransport } from "../../lib/api";

const sources = ["factory default", "user file"] as const;
type Source = (typeof sources)[number];
export type ProcessSetting<T> = Readonly<{ value: T; source: Source }>;
export type OverlayConfig = Readonly<{ name: string; repo: string; branch: string }>;
export type ProcessSettings = Readonly<{
  newsURL: ProcessSetting<string>;
  overlays: ProcessSetting<readonly OverlayConfig[]>;
  privateQueryPerMin: ProcessSetting<number>;
}>;

function setting<T>(payload: unknown, parse: (value: unknown) => T | undefined): ProcessSetting<T> | undefined {
  const field = objectFromPayload(payload);
  if (!field || !sources.includes(field.source as Source)) return undefined;
  const value = parse(field.value);
  return value === undefined ? undefined : { value, source: field.source as Source };
}

function overlaysFromPayload(value: unknown): readonly OverlayConfig[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const result: OverlayConfig[] = [];
  for (const item of value) {
    const overlay = objectFromPayload(item);
    if (!overlay || typeof overlay.name !== "string" || typeof overlay.repo !== "string" || typeof overlay.branch !== "string") return undefined;
    result.push({ name: overlay.name, repo: overlay.repo, branch: overlay.branch });
  }
  return result;
}

function processSettingsFromPayload(payload: unknown): ProcessSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) return undefined;
  const newsURL = setting(response.news_url, value => typeof value === "string" ? value : undefined);
  const overlays = setting(response.overlays, overlaysFromPayload);
  const privateQueryPerMin = setting(response.private_query_per_min, value =>
    typeof value === "number" && Number.isSafeInteger(value) && value > 0 ? value : undefined);
  return newsURL && overlays && privateQueryPerMin ? { newsURL, overlays, privateQueryPerMin } : undefined;
}

export function loadProcessSettings(transport: ApiTransport): Promise<ApiResult<ProcessSettings>> {
  return transport.request("/api/process/settings", { parse: processSettingsFromPayload });
}
