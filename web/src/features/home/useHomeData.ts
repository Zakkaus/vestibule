import { useCallback, useEffect, useMemo, useState } from "react";

import {
  consoleApi,
  retryConsoleAccess,
  type ConsoleRole,
  type ConsoleSessionState
} from "../../app/session";
import type { ApiRequestError, ApiResult } from "../../lib/api";
import {
  loadDiagnostics,
  type Diagnostics
} from "../diagnostics/api";
import {
  loadQueue,
  type QueueRecord
} from "../queue/api";
import {
  loadStats,
  type StatsQuery,
  type StatsReport
} from "../stats/api";
import {
  loadHomeSettings,
  type HomeSettings
} from "./api";

export type HomeDiagnosticsState =
  | Readonly<{ kind: "hidden" }>
  | Readonly<{ kind: "loaded"; diagnostics: Diagnostics }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

export type HomeData = Readonly<{
  queue: readonly QueueRecord[];
  stats: StatsReport;
  settings: HomeSettings;
  diagnostics: HomeDiagnosticsState;
}>;

export type HomeDataState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; data: HomeData }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

type HomeLoadResult = Readonly<{
  queue: ApiResult<readonly QueueRecord[]>;
  stats: ApiResult<StatsReport>;
  settings: ApiResult<HomeSettings>;
  diagnostics: ApiResult<Diagnostics> | null;
}>;

const pendingHomeLoads = new Map<string, Promise<HomeLoadResult>>();

function sharedHomeLoad(
  role: ConsoleRole,
  chatID: string,
  statsQuery: StatsQuery
): Promise<HomeLoadResult> {
  const key = [role, chatID, statsQuery.from, statsQuery.to, statsQuery.timezone].join("\u0000");
  const existing = pendingHomeLoads.get(key);
  if (existing) {
    return existing;
  }

  const diagnosticsRequest: Promise<ApiResult<Diagnostics> | null> = role === "operator"
    ? loadDiagnostics(consoleApi)
    : Promise.resolve(null);
  const request = Promise.all([
    loadQueue(consoleApi, chatID),
    loadStats(consoleApi, chatID, statsQuery),
    loadHomeSettings(consoleApi, chatID),
    diagnosticsRequest
  ]).then(([queue, stats, settings, diagnostics]) => ({
    queue,
    stats,
    settings,
    diagnostics
  }));
  pendingHomeLoads.set(key, request);
  void request.then(
    () => {
      if (pendingHomeLoads.get(key) === request) {
        pendingHomeLoads.delete(key);
      }
    },
    () => {
      if (pendingHomeLoads.get(key) === request) {
        pendingHomeLoads.delete(key);
      }
    }
  );
  return request;
}

function browserTimeZone(): string {
  try {
    const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    return timeZone.length > 0 ? timeZone : "UTC";
  } catch {
    return "UTC";
  }
}

function calendarDateInTimeZone(timeZone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit"
  }).formatToParts();
  const values: Record<string, string> = {};

  for (const part of parts) {
    if (part.type === "year" || part.type === "month" || part.type === "day") {
      values[part.type] = part.value;
    }
  }

  return values.year && values.month && values.day
    ? `${values.year}-${values.month}-${values.day}`
    : new Date().toISOString().slice(0, 10);
}

function shiftCalendarDate(date: string, days: number): string {
  const shifted = new Date(`${date}T00:00:00Z`);
  shifted.setUTCDate(shifted.getUTCDate() + days);
  return shifted.toISOString().slice(0, 10);
}

function homeStatsQuery(): StatsQuery {
  const timeZone = browserTimeZone();
  const today = calendarDateInTimeZone(timeZone);
  return {
    from: shiftCalendarDate(today, -6),
    to: shiftCalendarDate(today, 1),
    timezone: timeZone
  };
}

export function useHomeData(
  session: ConsoleSessionState,
  chatID: string | undefined
): Readonly<{ state: HomeDataState; reload: () => void }> {
  const [state, setState] = useState<HomeDataState>({ kind: "loading" });
  const [reloadVersion, setReloadVersion] = useState(0);
  const statsQuery = useMemo(homeStatsQuery, []);
  const reload = useCallback(() => {
    if (!retryConsoleAccess(session)) {
      setReloadVersion((version) => version + 1);
    }
  }, [session]);

  useEffect(() => {
    let active = true;

    if (session.state === "loading" || session.state === "checking-groups") {
      setState({ kind: "loading" });
      return () => {
        active = false;
      };
    }
    if (session.state === "blocked" || session.state === "groups-unavailable") {
      setState({ kind: "unavailable", error: session.error });
      return () => {
        active = false;
      };
    }
    if (session.state === "no-groups") {
      setState({ kind: "no-groups" });
      return () => {
        active = false;
      };
    }
    if (!chatID) {
      setState({ kind: "group-required" });
      return () => {
        active = false;
      };
    }

    setState({ kind: "loading" });
    void sharedHomeLoad(
      session.session.subject.role,
      chatID,
      statsQuery
    ).then(({ queue, stats, settings, diagnostics }) => {
      if (!active) {
        return;
      }
      if (!queue.ok) {
        setState({ kind: "unavailable", error: queue.error });
        return;
      }
      if (!stats.ok) {
        setState({ kind: "unavailable", error: stats.error });
        return;
      }
      if (!settings.ok) {
        setState({ kind: "unavailable", error: settings.error });
        return;
      }

      const diagnosticsState: HomeDiagnosticsState = diagnostics === null
        ? { kind: "hidden" }
        : diagnostics.ok
          ? { kind: "loaded", diagnostics: diagnostics.data }
          : { kind: "unavailable", error: diagnostics.error };
      setState({
        kind: "loaded",
        data: {
          queue: queue.data,
          stats: stats.data,
          settings: settings.data,
          diagnostics: diagnosticsState
        }
      });
    });

    return () => {
      active = false;
    };
  }, [chatID, reloadVersion, session, statsQuery]);

  return { state, reload };
}
