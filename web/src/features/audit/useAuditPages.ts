import { useEffect, useRef, useState } from "react";

import { consoleApi, type ConsoleSessionState } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import { loadAuditRecords, type AuditRecord } from "./api";
import type { AuditFixture } from "./fixtures";

export type AuditScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "fixture"; fixture: AuditFixture }>
  | Readonly<{ kind: "loaded" }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

function auditReadState(session: ConsoleSessionState, chatID: string | undefined, fixture: AuditFixture): AuditScreenState | null {
  if (session.state === "loading" || session.state === "checking-groups") return { kind: "loading" };
  if (session.state === "blocked" && session.error.kind === "non-json") return { kind: "fixture", fixture };
  if (session.state === "blocked" || session.state === "groups-unavailable") return { kind: "unavailable", error: session.error };
  if (session.state === "no-groups") return { kind: "no-groups" };
  if (!chatID) return { kind: "group-required" };
  return null;
}

export const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_not_found: true
};

export function useAuditPages(session: ConsoleSessionState, chatID: string | undefined, fixture: AuditFixture, reloadVersion: number) {
  const [auditState, setAuditState] = useState<AuditScreenState>({ kind: "loading" });
  const [records, setRecords] = useState<readonly AuditRecord[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [pageError, setPageError] = useState<ApiRequestError | null>(null);
  const activeScopeRef = useRef("");
  const sequenceRef = useRef(0);
  const pagingRef = useRef(false);

  useEffect(() => {
    const scope = `${session.state}:${chatID ?? ""}:${++sequenceRef.current}`;
    activeScopeRef.current = scope;
    let active = true;
    setRecords([]);
    setNextCursor(null);
    setPageError(null);
    setLoadingMore(false);
    pagingRef.current = false;
    const cleanup = () => {
      active = false;
      activeScopeRef.current = "";
    };
    const state = auditReadState(session, chatID, fixture);
    if (state) {
      setAuditState(state);
      if (state.kind === "fixture") setRecords([...fixture.records]);
      return cleanup;
    }
    setAuditState({ kind: "loading" });
    void loadAuditRecords(consoleApi, chatID!).then((result) => {
      if (!active || activeScopeRef.current !== scope) return;
      if (result.ok) {
        setRecords(result.data.records);
        setNextCursor(result.data.nextCursor);
        setAuditState({ kind: "loaded" });
      } else {
        setAuditState({ kind: "unavailable", error: result.error });
      }
    });
    return cleanup;
  }, [chatID, fixture, reloadVersion, session]);

  function loadMore(): void {
    if (!chatID || !nextCursor || pagingRef.current || auditState.kind !== "loaded") return;
    const scope = activeScopeRef.current;
    pagingRef.current = true;
    setLoadingMore(true);
    setPageError(null);
    void loadAuditRecords(consoleApi, chatID, nextCursor).then((result) => {
      if (activeScopeRef.current !== scope) return;
      pagingRef.current = false;
      setLoadingMore(false);
      if (result.ok) {
        setRecords((current) => {
          const ids = new Set(current.map((record) => record.id));
          return [...current, ...result.data.records.filter((record) => !ids.has(record.id))];
        });
        setNextCursor(result.data.nextCursor);
      } else {
        setPageError(result.error);
        if (result.error.kind === "api" && accessRevocationCodes[result.error.code]) {
          setRecords([]);
          setNextCursor(null);
          setAuditState({ kind: "unavailable", error: result.error });
        }
      }
    });
  }

  return { records, setRecords, auditState, setAuditState, activeScopeRef, nextCursor, loadingMore, pageError, loadMore };
}
