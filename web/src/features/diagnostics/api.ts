import {
  objectFromPayload,
  timestampFromPayload,
  type ApiResult,
  type ApiTransport
} from "../../lib/api";

export type DiagnosticsHealth = Readonly<{
  live: boolean;
  ready: boolean;
  configReady: boolean;
  telegramReady: boolean;
}>;

export type DiagnosticsBotAPI = Readonly<{
  lastHeartbeatAt: string | null;
  latencyMilliseconds: number | null;
}>;

export type DiagnosticsPersistence = Readonly<{
  configured: boolean;
  durable: boolean;
  writable: boolean;
  lastError: string | null;
}>;

export type Diagnostics = Readonly<{
  health: DiagnosticsHealth;
  botAPI: DiagnosticsBotAPI;
  persistence: DiagnosticsPersistence;
}>;

function booleanFromPayload(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function nullableTimestampFromPayload(value: unknown): string | null | undefined {
  return value === null ? null : timestampFromPayload(value);
}

function nullableLatencyFromPayload(value: unknown): number | null | undefined {
  if (value === null) {
    return null;
  }
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
    ? value
    : undefined;
}

function nullableStringFromPayload(value: unknown): string | null | undefined {
  return value === null || typeof value === "string" ? value : undefined;
}

function healthFromPayload(value: unknown): DiagnosticsHealth | undefined {
  const health = objectFromPayload(value);
  if (!health) {
    return undefined;
  }

  const live = booleanFromPayload(health.live);
  const ready = booleanFromPayload(health.ready);
  const configReady = booleanFromPayload(health.config_ready);
  const telegramReady = booleanFromPayload(health.telegram_ready);
  return live === undefined || ready === undefined || configReady === undefined || telegramReady === undefined
    ? undefined
    : { live, ready, configReady, telegramReady };
}

function botAPIFromPayload(value: unknown): DiagnosticsBotAPI | undefined {
  const botAPI = objectFromPayload(value);
  if (!botAPI) {
    return undefined;
  }

  const lastHeartbeatAt = nullableTimestampFromPayload(botAPI.last_heartbeat_at);
  const latencyMilliseconds = nullableLatencyFromPayload(botAPI.latency_ms);
  return lastHeartbeatAt === undefined || latencyMilliseconds === undefined
    ? undefined
    : { lastHeartbeatAt, latencyMilliseconds };
}

function persistenceFromPayload(value: unknown): DiagnosticsPersistence | undefined {
  const persistence = objectFromPayload(value);
  if (!persistence) {
    return undefined;
  }

  const configured = booleanFromPayload(persistence.configured);
  const durable = booleanFromPayload(persistence.durable);
  const writable = booleanFromPayload(persistence.writable);
  const lastError = nullableStringFromPayload(persistence.last_error);
  return configured === undefined || durable === undefined || writable === undefined || lastError === undefined
    ? undefined
    : { configured, durable, writable, lastError };
}

function diagnosticsFromPayload(payload: unknown): Diagnostics | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const health = healthFromPayload(response.health);
  const botAPI = botAPIFromPayload(response.bot_api);
  const persistence = persistenceFromPayload(response.persistence);
  return health && botAPI && persistence ? { health, botAPI, persistence } : undefined;
}

export function loadDiagnostics(transport: ApiTransport): Promise<ApiResult<Diagnostics>> {
  return transport.request("/api/status", { parse: diagnosticsFromPayload });
}
