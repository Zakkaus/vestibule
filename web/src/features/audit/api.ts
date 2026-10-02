import {
  nonEmptyStringFromPayload,
  objectFromPayload,
  timestampFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";
import {
  challengeResultFromPayload,
  type ChallengeResultDefinition
} from "../../lib/challenge";

export const auditUndoStates = [
  "unavailable",
  "available",
  "pending",
  "completed",
  "failed"
] as const;

export type AuditUndoState = (typeof auditUndoStates)[number];

export type AuditRecord = Readonly<{
  id: string;
  user: string;
  groupKey: string;
  groupLabelKey?: string;
  result: ChallengeResultDefinition;
  settledAt: string;
  settledBy: string | null;
  undoState: AuditUndoState;
}>;

export type AuditPage = Readonly<{
  records: readonly AuditRecord[];
  nextCursor: string | null;
}>;

function auditRecordFromPayload(payload: unknown): AuditRecord | undefined {
  const item = objectFromPayload(payload);
  if (!item || item.kind !== "challenge") {
    return undefined;
  }

  const id = nonEmptyStringFromPayload(item.id);
  const user = nonEmptyStringFromPayload(item.user);
  const groupKey = nonEmptyStringFromPayload(item.group_key);
  const result = challengeResultFromPayload(item.result);
  const settledAt = timestampFromPayload(item.settled_at);
  const settledBy = item.settled_by === null ? null : nonEmptyStringFromPayload(item.settled_by);
  const undoState = nonEmptyStringFromPayload(item.undo_state);

  if (
    !id ||
    !user ||
    !groupKey ||
    !result ||
    result.state === "pending" ||
    !settledAt ||
    settledBy === undefined ||
    !undoState ||
    !auditUndoStates.includes(undoState as AuditUndoState)
  ) {
    return undefined;
  }

  return {
    id,
    user,
    groupKey,
    result,
    settledAt,
    settledBy,
    undoState: undoState as AuditUndoState
  };
}

function auditRecordsFromPayload(payload: unknown): AuditPage | undefined {
  const response = objectFromPayload(payload);
  if (!response || !Array.isArray(response.items) ||
      (response.next_cursor !== null && !nonEmptyStringFromPayload(response.next_cursor))) {
    return undefined;
  }

  const records: AuditRecord[] = [];
  for (const item of response.items) {
    const record = auditRecordFromPayload(item);
    if (!record) {
      return undefined;
    }
    records.push(record);
  }
  return { records, nextCursor: response.next_cursor as string | null };
}

export function loadAuditRecords(
  transport: ApiTransport,
  chatID: string,
  cursor?: string
): Promise<ApiResult<AuditPage>> {
  const query = cursor ? `?cursor=${encodeURIComponent(cursor)}` : "";
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/audit${query}`, {
    parse: auditRecordsFromPayload
  });
}

export function undoAuditRecord(
  transport: ApiTransport,
  chatID: string,
  record: AuditRecord
): Promise<ApiResult<AuditRecord>> {
  return transport.request(
    `/api/chats/${encodeURIComponent(chatID)}/audit/${encodeURIComponent(record.id)}/undo`,
    {
      method: "POST",
      parse: auditRecordFromPayload
    }
  );
}
