import { objectFromPayload, type ApiTransport } from "../../lib/api";

export type KnownChats = Readonly<{
  revision: number;
  value: readonly number[];
  source: "factory default" | "user file" | "chat override";
}>;

function knownChatsFromPayload(payload: unknown): KnownChats | undefined {
  const response = objectFromPayload(payload);
  const setting = objectFromPayload(response?.known_chat_ids);
  if (!response || !Number.isSafeInteger(response.revision) || (response.revision as number) < 0 || !setting ||
      !Array.isArray(setting.value) || !setting.value.every(Number.isSafeInteger) ||
      !["factory default", "user file", "chat override"].includes(setting.source as string)) return undefined;
  return { revision: response.revision as number, value: setting.value, source: setting.source as KnownChats["source"] };
}

export function loadKnownChats(transport: ApiTransport, chatID: string) {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, { parse: knownChatsFromPayload });
}

export function saveKnownChats(transport: ApiTransport, chatID: string, revision: number, value: readonly number[] | null) {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    method: "PATCH",
    body: { expected_revision: revision, changes: { known_chat_ids: value } },
    parse: knownChatsFromPayload
  });
}

export function parseKnownChats(raw: string): readonly number[] | undefined {
  const lines = raw.split(/\s+/).filter(Boolean);
  if (lines.some(line => !/^-?\d+$/.test(line))) return undefined;
  const values = lines.map(Number);
  if (values.some(value => !Number.isSafeInteger(value) || value === 0) || new Set(values).size !== values.length) return undefined;
  return values;
}
