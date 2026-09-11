import { objectFromPayload, type ApiResult, type ApiTransport } from "../../lib/api";

export const ownerLimitFields = [
  "timeout_seconds",
  "ban_seconds",
  "mute_seconds",
  "lookup_ttl_seconds",
  "verify_retry_seconds",
  "verify_max_fails",
  "warn_limit",
  "private_query_per_min",
  "questions",
  "fallback_questions",
  "channel_whitelist",
  "trusted_member_group_ids",
  "known_chat_ids"
] as const;

export type OwnerLimitField = (typeof ownerLimitFields)[number];
export type OwnerLimits = Readonly<Record<OwnerLimitField, number>>;
export type OwnerLimitChanges = Partial<Record<OwnerLimitField, number | null>>;

export type OwnerLimitViolation = Readonly<{
  chat_id: string;
  field: OwnerLimitField;
  value: number;
  limit: number;
}>;

export type OwnerLimitsResponse = Readonly<{
  revision: number;
  limits: OwnerLimits;
  violations: readonly OwnerLimitViolation[];
}>;

function safeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function limitsFromPayload(value: unknown): OwnerLimits | undefined {
  const limits = objectFromPayload(value);
  if (!limits) return undefined;
  const result = {} as Record<OwnerLimitField, number>;
  for (const field of ownerLimitFields) {
    const limit = limits[field];
    if (!safeInteger(limit)) return undefined;
    result[field] = limit;
  }
  return result;
}

function violationFromPayload(value: unknown): OwnerLimitViolation | undefined {
  const violation = objectFromPayload(value);
  if (!violation || typeof violation.chat_id !== "string" || violation.chat_id.length === 0) {
    return undefined;
  }
  if (
    typeof violation.field !== "string" ||
    !(ownerLimitFields as readonly string[]).includes(violation.field) ||
    !safeInteger(violation.value) ||
    !safeInteger(violation.limit)
  ) {
    return undefined;
  }
  return {
    chat_id: violation.chat_id,
    field: violation.field as OwnerLimitField,
    value: violation.value,
    limit: violation.limit
  };
}

export function ownerLimitsFromPayload(payload: unknown): OwnerLimitsResponse | undefined {
  const response = objectFromPayload(payload);
  if (!response || !safeInteger(response.revision)) return undefined;
  const limits = limitsFromPayload(response.limits);
  if (!limits || !Array.isArray(response.violations)) return undefined;
  const violations: OwnerLimitViolation[] = [];
  for (const item of response.violations) {
    const violation = violationFromPayload(item);
    if (!violation) return undefined;
    violations.push(violation);
  }
  return { revision: response.revision, limits, violations };
}

export function loadOwnerLimits(transport: ApiTransport): Promise<ApiResult<OwnerLimitsResponse>> {
  return transport.request("/api/owner/limits", { parse: ownerLimitsFromPayload });
}

export function saveOwnerLimits(
  transport: ApiTransport,
  expectedRevision: number,
  changes: OwnerLimitChanges
): Promise<ApiResult<OwnerLimitsResponse>> {
  return transport.request("/api/owner/limits", {
    method: "PATCH",
    body: { expected_revision: expectedRevision, changes },
    parse: ownerLimitsFromPayload
  });
}
