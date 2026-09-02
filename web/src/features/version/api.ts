import {
  nonEmptyStringFromPayload,
  objectFromPayload,
  timestampFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export type ReplacementResult = Readonly<{
  status: string;
  requestedVersion: string;
  reason: string;
}>;

export type VersionStatus = Readonly<{
  version: string;
  replacement: Readonly<{
    unitAvailable: boolean;
    lastResult: ReplacementResult | null;
  }>;
}>;

export type ReleaseRollback = Readonly<{
  available: boolean;
  reason: string;
  targetSchemaVersion: number;
  retainedSchemaVersion: number;
  minimumRollbackSchemaVersion: number;
}>;

export type LatestRelease = Readonly<{
  version: string;
  url: string;
  notes: string;
  publishedAt: string;
  updateAvailable: boolean;
  rollback: ReleaseRollback | null;
}>;

function replacementResultFromPayload(value: unknown): ReplacementResult | null | undefined {
  if (value === null) {
    return null;
  }
  const result = objectFromPayload(value);
  if (!result) {
    return undefined;
  }
  const status = nonEmptyStringFromPayload(result.status);
  const requestedVersion = nonEmptyStringFromPayload(result.requested_version);
  const reason = typeof result.reason === "string" ? result.reason : undefined;
  return status && requestedVersion && reason !== undefined
    ? { status, requestedVersion, reason }
    : undefined;
}

function versionStatusFromPayload(payload: unknown): VersionStatus | undefined {
  const response = objectFromPayload(payload);
  const replacement = objectFromPayload(response?.replacement);
  if (!response || !replacement) {
    return undefined;
  }
  const version = nonEmptyStringFromPayload(response.version);
  const unitAvailable = replacement.unit_available;
  const lastResult = replacementResultFromPayload(replacement.last_result);
  return version && typeof unitAvailable === "boolean" && lastResult !== undefined
    ? { version, replacement: { unitAvailable, lastResult } }
    : undefined;
}

function positiveIntegerFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0
    ? value
    : undefined;
}

function releaseURLFromPayload(value: unknown): string | undefined {
  const raw = nonEmptyStringFromPayload(value);
  if (!raw) {
    return undefined;
  }
  try {
    const parsed = new URL(raw);
    const releasePath = "/Zakkaus/vestibule/releases/tag/";
    return parsed.protocol === "https:" && parsed.hostname === "github.com" &&
      parsed.pathname.startsWith(releasePath)
      ? parsed.toString()
      : undefined;
  } catch {
    return undefined;
  }
}

function rollbackFromPayload(value: unknown): ReleaseRollback | null | undefined {
  if (value === null) {
    return null;
  }
  const rollback = objectFromPayload(value);
  if (!rollback) {
    return undefined;
  }
  const targetSchemaVersion = positiveIntegerFromPayload(rollback.target_schema_version);
  const retainedSchemaVersion = positiveIntegerFromPayload(rollback.retained_schema_version);
  const minimumRollbackSchemaVersion = positiveIntegerFromPayload(
    rollback.minimum_rollback_schema_version
  );
  return typeof rollback.available === "boolean" && typeof rollback.reason === "string" &&
    targetSchemaVersion && retainedSchemaVersion && minimumRollbackSchemaVersion
    ? {
        available: rollback.available,
        reason: rollback.reason,
        targetSchemaVersion,
        retainedSchemaVersion,
        minimumRollbackSchemaVersion
      }
    : undefined;
}

function releaseFromPayload(payload: unknown): LatestRelease | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }
  const version = nonEmptyStringFromPayload(response.version);
  const url = releaseURLFromPayload(response.url);
  const notes = typeof response.notes === "string" ? response.notes : undefined;
  const publishedAt = timestampFromPayload(response.published_at);
  const rollback = rollbackFromPayload(response.rollback);
  const updateAvailable = response.update_available;
  if (!version || !url || notes === undefined || !publishedAt ||
      typeof updateAvailable !== "boolean" || rollback === undefined ||
      (updateAvailable && rollback === null)) {
    return undefined;
  }
  return { version, url, notes, publishedAt, updateAvailable, rollback };
}

function upgradeResponseFromPayload(payload: unknown): "requested" | undefined {
  const response = objectFromPayload(payload);
  return response?.status === "requested" ? "requested" : undefined;
}

export function loadVersionStatus(transport: ApiTransport): Promise<ApiResult<VersionStatus>> {
  return transport.request("/api/status", { parse: versionStatusFromPayload });
}

export function loadLatestRelease(transport: ApiTransport): Promise<ApiResult<LatestRelease>> {
  return transport.request("/api/status/release", { parse: releaseFromPayload });
}

export function requestUpgrade(
  transport: ApiTransport,
  version: string
): Promise<ApiResult<"requested">> {
  return transport.request("/api/status/upgrade", {
    method: "POST",
    body: { version },
    parse: upgradeResponseFromPayload
  });
}
