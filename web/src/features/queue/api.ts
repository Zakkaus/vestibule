import type { ApiResult, ApiTransport } from "../../lib/api";
import {
  challengeResults,
  type ChallengeResultDefinition,
  type ChallengeState,
  type DeclineReason
} from "../../lib/challenge";

export type QueueRecord = Readonly<{
  id: string;
  user: string;
  groupKey: string;
  groupLabelKey?: string;
  result: ChallengeResultDefinition;
  occurredAt: string | null;
  expiresAt: string;
  remainingSeconds?: number;
}>;

const settledResults = {
  pending: challengeResults.pending,
  approved: challengeResults.approved,
  banned: challengeResults.banned,
  expired: challengeResults.expired,
  superseded: challengeResults.superseded
} as const satisfies Readonly<
  Record<Exclude<ChallengeState, "declined">, ChallengeResultDefinition>
>;

const declinedResults = {
  wrong_answer: challengeResults.declinedWrongAnswer,
  rejected: challengeResults.declinedRejected,
  external_unmet: challengeResults.declinedExternalUnmet
} as const satisfies Readonly<Record<DeclineReason, ChallengeResultDefinition>>;

function objectFrom(value: unknown): Readonly<Record<string, unknown>> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Readonly<Record<string, unknown>>)
    : undefined;
}

function nonEmptyString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function timestamp(value: unknown): string | undefined {
  const parsed = nonEmptyString(value);
  return parsed !== undefined && Number.isFinite(Date.parse(parsed)) ? parsed : undefined;
}

function resultFromPayload(value: unknown): ChallengeResultDefinition | undefined {
  const result = objectFrom(value);
  const state = result ? nonEmptyString(result.state) : undefined;

  if (!result || !state || !("reason" in result)) {
    return undefined;
  }

  if (state === "declined") {
    const reason = result.reason;
    return typeof reason === "string" && reason in declinedResults
      ? declinedResults[reason as DeclineReason]
      : undefined;
  }

  return result.reason === null && state in settledResults
    ? settledResults[state as Exclude<ChallengeState, "declined">]
    : undefined;
}

function queueRecordFromPayload(payload: unknown): QueueRecord | undefined {
  const item = objectFrom(payload);
  if (!item) {
    return undefined;
  }

  const id = nonEmptyString(item.id);
  const user = nonEmptyString(item.user);
  const groupKey = nonEmptyString(item.group_key);
  const result = resultFromPayload(item.result);
  const expiresAt = timestamp(item.expires_at);
  const occurredAt = item.occurred_at === null ? null : timestamp(item.occurred_at);
  const remainingSeconds = item.remaining_seconds;

  if (
    !id ||
    !user ||
    !groupKey ||
    !result ||
    !expiresAt ||
    occurredAt === undefined ||
    (remainingSeconds !== null &&
      (typeof remainingSeconds !== "number" ||
        !Number.isSafeInteger(remainingSeconds) ||
        remainingSeconds < 0))
  ) {
    return undefined;
  }

  return {
    id,
    user,
    groupKey,
    result,
    occurredAt,
    expiresAt,
    ...(typeof remainingSeconds === "number" ? { remainingSeconds } : {})
  };
}

function queueRecordsFromPayload(payload: unknown): readonly QueueRecord[] | undefined {
  const response = objectFrom(payload);
  if (!response || !Array.isArray(response.items)) {
    return undefined;
  }

  const records: QueueRecord[] = [];
  for (const item of response.items) {
    const record = queueRecordFromPayload(item);
    if (!record) {
      return undefined;
    }
    records.push(record);
  }
  return records;
}

export function loadQueue(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<readonly QueueRecord[]>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/queue`, {
    parse: queueRecordsFromPayload
  });
}

export function releaseQueueRecord(
  transport: ApiTransport,
  chatID: string,
  record: QueueRecord
): Promise<ApiResult<QueueRecord>> {
  return transport.request(
    `/api/chats/${encodeURIComponent(chatID)}/queue/${encodeURIComponent(record.id)}`,
    {
      method: "POST",
      body: {
        expected: {
          state: record.result.state,
          reason: record.result.reason ?? ""
        },
        result: {
          state: "approved",
          reason: ""
        }
      },
      parse: queueRecordFromPayload
    }
  );
}
