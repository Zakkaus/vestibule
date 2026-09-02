import {
  nonEmptyStringFromPayload,
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

export type DiagnosticsRejectionReason = Readonly<{
  reason: string | null;
  count: number;
}>;

export type DiagnosticsRejections = Readonly<{
  sourceAvailable: boolean;
  humanReviewRequired: boolean;
  windowSeconds: number;
  byReason: readonly DiagnosticsRejectionReason[];
}>;

export type DiagnosticsProblemStreak = Readonly<{
  thresholdSeconds: number;
  firstProblemAt: string | null;
  lastProblemAt: string | null;
  lastRecoveredAt: string | null;
  problemSpanSeconds: number;
  exceedsThreshold: boolean;
}>;

export type DiagnosticsChallengeDelivery = Readonly<{
  streak: DiagnosticsProblemStreak;
  failedDeliveries: number;
  duplicateDeliveries: number;
}>;

export type DiagnosticsConsoleAccess = Readonly<{
  streak: DiagnosticsProblemStreak;
  unavailableAttempts: number;
}>;

export type DiagnosticsDatabaseWrites = Readonly<{
  scope: string;
  windowSeconds: number;
  totalWrites: number;
  failedWrites: number;
  failureRatePercent: number;
  exceedsOnePercent: boolean;
}>;

export type DiagnosticsRollback = Readonly<{
  rejections: DiagnosticsRejections;
  challengeDelivery: DiagnosticsChallengeDelivery;
  consoleAccess: DiagnosticsConsoleAccess;
  databaseWrites: DiagnosticsDatabaseWrites;
}>;

export type Diagnostics = Readonly<{
  health: DiagnosticsHealth;
  botAPI: DiagnosticsBotAPI;
  persistence: DiagnosticsPersistence;
  rollback: DiagnosticsRollback | null;
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

function countFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

function measureFromPayload(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : undefined;
}

function rejectionReasonFromPayload(payload: unknown): DiagnosticsRejectionReason | undefined {
  const entry = objectFromPayload(payload);
  if (!entry) {
    return undefined;
  }

  const reason = entry.reason === null ? null : nonEmptyStringFromPayload(entry.reason);
  const count = countFromPayload(entry.count);
  return reason === undefined || count === undefined ? undefined : { reason, count };
}

function rejectionsFromPayload(value: unknown): DiagnosticsRejections | undefined {
  const rejections = objectFromPayload(value);
  if (!rejections || !Array.isArray(rejections.by_reason)) {
    return undefined;
  }

  const byReason: DiagnosticsRejectionReason[] = [];
  for (const item of rejections.by_reason) {
    const entry = rejectionReasonFromPayload(item);
    if (!entry) {
      return undefined;
    }
    byReason.push(entry);
  }

  const sourceAvailable = booleanFromPayload(rejections.source_available);
  const humanReviewRequired = booleanFromPayload(rejections.human_review_required);
  const windowSeconds = countFromPayload(rejections.window_seconds);
  return sourceAvailable === undefined ||
    humanReviewRequired === undefined ||
    windowSeconds === undefined
    ? undefined
    : { sourceAvailable, humanReviewRequired, windowSeconds, byReason };
}

function problemStreakFromPayload(value: unknown): DiagnosticsProblemStreak | undefined {
  const streak = objectFromPayload(value);
  if (!streak) {
    return undefined;
  }

  const thresholdSeconds = countFromPayload(streak.threshold_seconds);
  const firstProblemAt = nullableTimestampFromPayload(streak.first_problem_at);
  const lastProblemAt = nullableTimestampFromPayload(streak.last_problem_at);
  const lastRecoveredAt = nullableTimestampFromPayload(streak.last_recovered_at);
  const problemSpanSeconds = measureFromPayload(streak.problem_span_seconds);
  const exceedsThreshold = booleanFromPayload(streak.exceeds_threshold);
  return thresholdSeconds === undefined ||
    firstProblemAt === undefined ||
    lastProblemAt === undefined ||
    lastRecoveredAt === undefined ||
    problemSpanSeconds === undefined ||
    exceedsThreshold === undefined
    ? undefined
    : {
        thresholdSeconds,
        firstProblemAt,
        lastProblemAt,
        lastRecoveredAt,
        problemSpanSeconds,
        exceedsThreshold
      };
}

function challengeDeliveryFromPayload(value: unknown): DiagnosticsChallengeDelivery | undefined {
  const delivery = objectFromPayload(value);
  if (!delivery) {
    return undefined;
  }

  const streak = problemStreakFromPayload(delivery.streak);
  const failedDeliveries = countFromPayload(delivery.failed_deliveries);
  const duplicateDeliveries = countFromPayload(delivery.duplicate_deliveries);
  return !streak || failedDeliveries === undefined || duplicateDeliveries === undefined
    ? undefined
    : { streak, failedDeliveries, duplicateDeliveries };
}

function consoleAccessFromPayload(value: unknown): DiagnosticsConsoleAccess | undefined {
  const access = objectFromPayload(value);
  if (!access) {
    return undefined;
  }

  const streak = problemStreakFromPayload(access.streak);
  const unavailableAttempts = countFromPayload(access.unavailable_attempts);
  return !streak || unavailableAttempts === undefined
    ? undefined
    : { streak, unavailableAttempts };
}

function databaseWritesFromPayload(value: unknown): DiagnosticsDatabaseWrites | undefined {
  const writes = objectFromPayload(value);
  if (!writes) {
    return undefined;
  }

  const scope = nonEmptyStringFromPayload(writes.scope);
  const windowSeconds = countFromPayload(writes.window_seconds);
  const totalWrites = countFromPayload(writes.total_writes);
  const failedWrites = countFromPayload(writes.failed_writes);
  const failureRatePercent = measureFromPayload(writes.failure_rate_percent);
  const exceedsOnePercent = booleanFromPayload(writes.exceeds_one_percent);
  return scope === undefined ||
    windowSeconds === undefined ||
    totalWrites === undefined ||
    failedWrites === undefined ||
    failureRatePercent === undefined ||
    exceedsOnePercent === undefined
    ? undefined
    : {
        scope,
        windowSeconds,
        totalWrites,
        failedWrites,
        failureRatePercent,
        exceedsOnePercent
      };
}

// A missing rollback_observations object is not a broken response. The readings
// exist so a cutover can be reverted, and reverting means running a binary that
// predates them; refusing the whole payload would blank the diagnostics screen
// at exactly the moment someone needs the rest of it. A malformed object is a
// different thing and is still refused.
function rollbackFromPayload(value: unknown): DiagnosticsRollback | null | undefined {
  if (value === undefined) {
    return null;
  }

  const rollback = objectFromPayload(value);
  if (!rollback) {
    return undefined;
  }

  const rejections = rejectionsFromPayload(rollback.rejections);
  const challengeDelivery = challengeDeliveryFromPayload(rollback.challenge_delivery);
  const consoleAccess = consoleAccessFromPayload(rollback.console_access);
  const databaseWrites = databaseWritesFromPayload(rollback.database_writes);
  return rejections && challengeDelivery && consoleAccess && databaseWrites
    ? { rejections, challengeDelivery, consoleAccess, databaseWrites }
    : undefined;
}

function diagnosticsFromPayload(payload: unknown): Diagnostics | undefined {
  const response = objectFromPayload(payload);
  if (!response) {
    return undefined;
  }

  const health = healthFromPayload(response.health);
  const botAPI = botAPIFromPayload(response.bot_api);
  const persistence = persistenceFromPayload(response.persistence);
  const rollback = rollbackFromPayload(response.rollback_observations);
  return health && botAPI && persistence && rollback !== undefined
    ? { health, botAPI, persistence, rollback }
    : undefined;
}

export function loadDiagnostics(transport: ApiTransport): Promise<ApiResult<Diagnostics>> {
  return transport.request("/api/status", { parse: diagnosticsFromPayload });
}
