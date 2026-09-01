import {
  objectFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export const messageSettingFields = [
  "name_spoiler",
  "lookup_auto_delete_enabled",
  "lookup_ttl_seconds",
  "rich_messages"
] as const;

export type MessageSettingField = (typeof messageSettingFields)[number];

export const settingSources = ["factory default", "user file", "chat override"] as const;
export type SettingSource = (typeof settingSources)[number];

export type SettingValue<T> = Readonly<{
  value: T;
  source: SettingSource;
}>;

export type MessageSettings = Readonly<{
  revision: number;
  name_spoiler: SettingValue<boolean>;
  lookup_auto_delete_enabled: SettingValue<boolean>;
  lookup_ttl_seconds: SettingValue<number>;
  rich_messages: SettingValue<boolean>;
}>;

export type MessageSettingsValues = Readonly<{
  name_spoiler: boolean;
  lookup_auto_delete_enabled: boolean;
  lookup_ttl_seconds: number;
  rich_messages: boolean;
}>;

export type MessageSettingsChanges = Partial<{
  name_spoiler: boolean | null;
  lookup_auto_delete_enabled: boolean | null;
  lookup_ttl_seconds: number | null;
  rich_messages: boolean | null;
}>;

export type RuleDefinition =
  | null
  | boolean
  | number
  | string
  | readonly RuleDefinition[]
  | RuleDefinitionObject;

export interface RuleDefinitionObject {
  readonly [key: string]: RuleDefinition;
}

export type MessageRule = Readonly<{
  id: string;
  collection: string;
  ordinal: number;
  enabled: boolean;
  definition: RuleDefinition;
}>;


function isSettingSource(value: unknown): value is SettingSource {
  return typeof value === "string" && settingSources.includes(value as SettingSource);
}

function nonEmptyString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function safeInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) ? value : undefined;
}


function settingFromPayload<T>(
  value: unknown,
  parseValue: (candidate: unknown) => T | undefined
): SettingValue<T> | undefined {
  const setting = objectFromPayload(value);
  if (!setting) {
    return undefined;
  }

  const parsedValue = parseValue(setting.value);
  const source = setting.source;
  if (parsedValue === undefined || !isSettingSource(source)) {
    return undefined;
  }

  return { value: parsedValue, source };
}

export function messageSettingsFromPayload(payload: unknown): MessageSettings | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const revision = safeInteger(response.revision);
  const nameSpoiler = settingFromPayload(response.name_spoiler, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const autoDeleteEnabled = settingFromPayload(response.lookup_auto_delete_enabled, (value) =>
    typeof value === "boolean" ? value : undefined
  );
  const ttlSeconds = settingFromPayload(response.lookup_ttl_seconds, safeInteger);
  const richMessages = settingFromPayload(response.rich_messages, (value) =>
    typeof value === "boolean" ? value : undefined
  );

  if (
    revision === undefined ||
    revision < 0 ||
    !nameSpoiler ||
    !autoDeleteEnabled ||
    !ttlSeconds ||
    !richMessages
  ) {
    return undefined;
  }

  return {
    revision,
    name_spoiler: nameSpoiler,
    lookup_auto_delete_enabled: autoDeleteEnabled,
    lookup_ttl_seconds: ttlSeconds,
    rich_messages: richMessages
  };
}


export function messageRuleFromPayload(payload: unknown): MessageRule | undefined {
  const item = objectFromPayload(payload);
  if (!item || !Object.prototype.hasOwnProperty.call(item, "definition")) {
    return undefined;
  }

  const id = nonEmptyString(item.id);
  const collection = nonEmptyString(item.collection);
  const ordinal = safeInteger(item.ordinal);
  const definition = item.definition as RuleDefinition;
  if (
    !id ||
    !collection ||
    ordinal === undefined ||
    ordinal < 0 ||
    typeof item.enabled !== "boolean"
  ) {
    return undefined;
  }

  return { id, collection, ordinal, enabled: item.enabled, definition };
}

function messageRulesFromPayload(payload: unknown): readonly MessageRule[] | undefined {
  const response = objectFromPayload(payload);
  if (!response || !Array.isArray(response.items)) {
    return undefined;
  }

  const items: MessageRule[] = [];
  for (const candidate of response.items) {
    const item = messageRuleFromPayload(candidate);
    if (!item) {
      return undefined;
    }
    items.push(item);
  }
  return items;
}


function rulesPath(chatID: string): string {
  return `/api/chats/${encodeURIComponent(chatID)}/rules`;
}

export function loadMessageSettings(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<MessageSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    parse: messageSettingsFromPayload
  });
}

export function saveMessageSettings(
  transport: ApiTransport,
  chatID: string,
  expectedRevision: number,
  changes: MessageSettingsChanges
): Promise<ApiResult<MessageSettings>> {
  return transport.request(`/api/chats/${encodeURIComponent(chatID)}/settings`, {
    method: "PATCH",
    body: { expected_revision: expectedRevision, changes },
    parse: messageSettingsFromPayload
  });
}

export function loadMessageRules(
  transport: ApiTransport,
  chatID: string
): Promise<ApiResult<readonly MessageRule[]>> {
  return transport.request(rulesPath(chatID), { parse: messageRulesFromPayload });
}

export function updateMessageRule(
  transport: ApiTransport,
  chatID: string,
  expected: MessageRule,
  item: MessageRule
): Promise<ApiResult<MessageRule>> {
  return transport.request(`${rulesPath(chatID)}/${encodeURIComponent(expected.id)}`, {
    method: "PUT",
    body: {
      expected: {
        collection: expected.collection,
        ordinal: expected.ordinal,
        enabled: expected.enabled,
        definition: expected.definition
      },
      item: {
        collection: item.collection,
        ordinal: item.ordinal,
        enabled: item.enabled,
        definition: item.definition
      }
    },
    parse: messageRuleFromPayload
  });
}

export function replaceMessageRuleCollection(
  transport: ApiTransport,
  chatID: string,
  collection: string,
  expected: readonly MessageRule[],
  items: readonly MessageRule[]
): Promise<ApiResult<readonly MessageRule[]>> {
  return transport.request(rulesPath(chatID), {
    method: "PUT",
    body: {
      collection,
      expected: expected.map((rule) => ({
        id: rule.id,
        enabled: rule.enabled,
        definition: rule.definition
      })),
      items: items.map((rule) => ({
        id: rule.id,
        enabled: rule.enabled,
        definition: rule.definition
      }))
    },
    parse: messageRulesFromPayload
  });
}
