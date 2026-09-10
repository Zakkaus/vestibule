import { useCallback, useEffect, useRef, useState } from "react";

import { consoleApi } from "../../app/session";
import type { ApiRequestError } from "../../lib/api";
import {
  loadLatestRelease,
  loadVersionStatus,
  requestUpgrade,
  type LatestRelease,
  type ReplacementResult,
  type VersionStatus
} from "./api";

const replacementPollIntervalMilliseconds = 2_000;
const replacementMonitorWindowMilliseconds = 5 * 60_000;
const failedReplacementStatuses: Readonly<Record<string, true>> = {
  failed: true,
  rejected: true,
  rolled_back: true,
  rollback_failed: true
};

export type VersionBaseState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "loaded"; status: VersionStatus }>
  | Readonly<{ kind: "access-denied" }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

export type VersionReleaseState =
  | Readonly<{ kind: "idle" }>
  | Readonly<{ kind: "checking" }>
  | Readonly<{ kind: "loaded"; release: LatestRelease }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>;

export type VersionUpgradeState =
  | Readonly<{ kind: "idle" }>
  | Readonly<{ kind: "confirming"; version: string }>
  | Readonly<{ kind: "requesting"; version: string }>
  | Readonly<{ kind: "monitoring"; version: string }>
  | Readonly<{ kind: "applied"; result: ReplacementResult }>
  | Readonly<{ kind: "failed"; result: ReplacementResult }>
  | Readonly<{ kind: "request-unavailable"; version: string; error: ApiRequestError }>
  | Readonly<{ kind: "monitor-unavailable"; version: string }>;

export type VersionController = Readonly<{
  baseState: VersionBaseState;
  releaseState: VersionReleaseState;
  upgradeState: VersionUpgradeState;
  reloadStatus: () => void;
  checkLatest: () => void;
  beginUpgrade: (version: string) => void;
  cancelUpgrade: () => void;
  confirmUpgrade: () => void;
  retryUpgrade: () => void;
}>;

export function useVersionController(): VersionController {
  const [baseState, setBaseState] = useState<VersionBaseState>({ kind: "loading" });
  const [releaseState, setReleaseState] = useState<VersionReleaseState>({ kind: "idle" });
  const [upgradeState, setUpgradeState] = useState<VersionUpgradeState>({ kind: "idle" });
  const [statusReload, setStatusReload] = useState(0);
  const mountedRef = useRef(true);
  const releasePendingRef = useRef(false);
  const upgradePendingRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    let active = true;
    setBaseState({ kind: "loading" });
    void loadVersionStatus(consoleApi).then((result) => {
      if (!active) {
        return;
      }
      if (result.ok) {
        setBaseState({ kind: "loaded", status: result.data });
      } else if (result.error.kind === "api" && result.error.code === "diagnostics_access_denied") {
        setBaseState({ kind: "access-denied" });
      } else {
        setBaseState({ kind: "unavailable", error: result.error });
      }
    });
    return () => {
      active = false;
    };
  }, [statusReload]);

  const checkLatest = useCallback(() => {
    if (releasePendingRef.current) {
      return;
    }
    releasePendingRef.current = true;
    setReleaseState({ kind: "checking" });
    void loadLatestRelease(consoleApi).then((result) => {
      releasePendingRef.current = false;
      if (!mountedRef.current) {
        return;
      }
      setReleaseState(result.ok
        ? { kind: "loaded", release: result.data }
        : { kind: "unavailable", error: result.error });
    });
  }, []);

  const beginUpgrade = useCallback((version: string) => {
    setUpgradeState((current) =>
      current.kind === "idle" ||
      current.kind === "request-unavailable" ||
      current.kind === "failed"
        ? { kind: "confirming", version }
        : current
    );
  }, []);

  const cancelUpgrade = useCallback(() => {
    if (!upgradePendingRef.current) {
      setUpgradeState({ kind: "idle" });
    }
  }, []);

  const submitUpgradeRequest = useCallback(async (version: string): Promise<void> => {
    const result = await requestUpgrade(consoleApi, version);
    upgradePendingRef.current = false;
    if (!mountedRef.current) {
      return;
    }
    setUpgradeState(result.ok
      ? { kind: "monitoring", version }
      : { kind: "request-unavailable", version, error: result.error });
  }, []);

  const confirmUpgrade = useCallback(() => {
    if (upgradeState.kind !== "confirming" || upgradePendingRef.current) {
      return;
    }
    upgradePendingRef.current = true;
    setUpgradeState({ kind: "requesting", version: upgradeState.version });
    void submitUpgradeRequest(upgradeState.version);
  }, [submitUpgradeRequest, upgradeState]);

  useEffect(() => {
    if (upgradeState.kind !== "monitoring") {
      return undefined;
    }
    let active = true;
    let timer: number | undefined;
    const { version } = upgradeState;
    const deadline = Date.now() + replacementMonitorWindowMilliseconds;

    const poll = async () => {
      const result = await loadVersionStatus(consoleApi);
      if (!active) {
        return;
      }
      if (result.ok) {
        setBaseState({ kind: "loaded", status: result.data });
        const replacement = result.data.replacement.lastResult;
        if (replacement?.requestedVersion === version && replacement.status === "applied") {
          setUpgradeState({ kind: "applied", result: replacement });
          return;
        }
        if (replacement?.requestedVersion === version && failedReplacementStatuses[replacement.status]) {
          setUpgradeState({ kind: "failed", result: replacement });
          return;
        }
      }
      if (Date.now() >= deadline) {
        setUpgradeState({ kind: "monitor-unavailable", version });
        return;
      }
      timer = window.setTimeout(() => void poll(), replacementPollIntervalMilliseconds);
    };

    void poll();
    return () => {
      active = false;
      if (timer !== undefined) {
        window.clearTimeout(timer);
      }
    };
  }, [upgradeState]);

  const retryUpgrade = useCallback(() => {
    setUpgradeState((current) => {
      if (current.kind === "request-unavailable") {
        return current.error.kind === "api"
          ? { kind: "confirming", version: current.version }
          : { kind: "monitoring", version: current.version };
      }
      if (current.kind === "monitor-unavailable") {
        return { kind: "monitoring", version: current.version };
      }
      return current;
    });
  }, []);

  return {
    baseState,
    releaseState,
    upgradeState,
    reloadStatus() {
      setStatusReload((version) => version + 1);
    },
    checkLatest,
    beginUpgrade,
    cancelUpgrade,
    confirmUpgrade,
    retryUpgrade
  };
}
