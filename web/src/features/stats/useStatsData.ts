import { useEffect, useState } from "react";

import { consoleApi, type ConsoleSessionState } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import {
  loadStats,
  type StatsQuery,
  type StatsReport
} from "./api";

export type StatsDataState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; report: StatsReport }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

export function useStatsData(
  session: ConsoleSessionState,
  chatID: string | undefined,
  query: StatsQuery,
  reloadVersion: number,
  onSettled: () => void
): StatsDataState {
  const [state, setState] = useState<StatsDataState>({ kind: "loading" });

  useEffect(() => {
    let active = true;

    if (session.state === "loading" || session.state === "checking-groups") {
      setState({ kind: "loading" });
      onSettled();
      return () => {
        active = false;
      };
    }
    if (session.state === "blocked" || session.state === "groups-unavailable") {
      setState({ kind: "unavailable", error: session.error });
      onSettled();
      return () => {
        active = false;
      };
    }
    if (session.state === "no-groups") {
      setState({ kind: "no-groups" });
      onSettled();
      return () => {
        active = false;
      };
    }
    if (!chatID) {
      setState({ kind: "group-required" });
      onSettled();
      return () => {
        active = false;
      };
    }

    setState({ kind: "loading" });
    void loadStats(consoleApi, chatID, query).then((result) => {
      if (!active) {
        return;
      }
      setState(result.ok ? { kind: "loaded", report: result.data } : { kind: "unavailable", error: result.error });
      onSettled();
    });

    return () => {
      active = false;
    };
  }, [chatID, onSettled, query, reloadVersion, session]);

  return state;
}
