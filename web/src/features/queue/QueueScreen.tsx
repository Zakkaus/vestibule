import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import { consoleApi, useConsoleSession } from "../../app/session";
import type { StatusTone } from "../../components/StatusBadge";
import type { ApiRequestError } from "../../lib/api";
import { Icon } from "../../icons";
import { challengeResults } from "../../lib/challenge";
import { loadQueue, releaseQueueRecord, type QueueRecord } from "./api";
import { queueFixtureFor, type QueueFilter, type QueueFixture } from "./fixtures";
import { QueueTable, type PendingQueueActions } from "./QueueTable";

const FIXTURE_ACTION_DELAY_MS = 700;
const FEEDBACK_DURATION_MS = 5_000;

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "queue.errors.authenticationExpired",
  authentication_invalid: "queue.errors.authenticationInvalid",
  chat_access_denied: "queue.errors.accessDenied",
  chat_access_unavailable: "queue.errors.accessUnavailable",
  chat_not_found: "queue.errors.chatNotFound",
  challenge_conflict: "queue.errors.challengeConflict",
  csrf_invalid: "queue.errors.csrfInvalid",
  invalid_settlement: "queue.errors.invalidSettlement",
  queue_unavailable: "queue.errors.queueUnavailable",
  settlement_unavailable: "queue.errors.settlementUnavailable",
  target_protected: "queue.errors.targetProtected",
  target_unavailable: "queue.errors.targetUnavailable"
};

const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_not_found: true
};

type QueueScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "fixture"; fixture: QueueFixture }>
  | Readonly<{ kind: "loaded" }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

type QueueFeedback = Readonly<{
  id: number;
  messageKey: string;
  tone: StatusTone;
  record: QueueRecord;
}>;

function queueErrorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "queue.errors.network";
  }

  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? fallback;
  }

  return fallback;
}

function QueueFeedbackNotice({ feedback }: Readonly<{ feedback: QueueFeedback }>) {
  const { t } = useTranslation();
  const group = feedback.record.groupLabelKey
    ? t(feedback.record.groupLabelKey)
    : feedback.record.groupKey;

  return (
    <div
      data-record-feedback
      data-queue-feedback
      data-tone={feedback.tone}
      role={feedback.tone === "error" ? "alert" : "status"}
      aria-atomic="true"
    >
      <Icon
        name={
          feedback.tone === "ok"
            ? "circleCheck"
            : feedback.tone === "info"
              ? "info"
              : feedback.tone === "pending"
                ? "loaderCircle"
                : feedback.tone === "error"
                  ? "circleAlert"
                  : "circleMinus"
        }
      />
      {t(feedback.messageKey, {
        user: feedback.record.user,
        group,
        approved: t(challengeResults.approved.labelKey)
      })}
    </div>
  );
}

function QueueEmptyState() {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-record-empty data-queue-empty aria-labelledby="queue-empty-title">
      <h2 id="queue-empty-title" data-state-heading>
        <Icon name="inbox" />
        {t("queue.empty.title")}
      </h2>
      <p>{t("queue.empty.description")}</p>
    </section>
  );
}

function QueueLoadingState() {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-record-empty data-queue-empty aria-live="polite" aria-labelledby="queue-loading-title">
      <h2 id="queue-loading-title" data-state-heading>
        <Icon name="loaderCircle" />
        {t("queue.loading.title")}
      </h2>
      <p>{t("queue.loading.description")}</p>
    </section>
  );
}

function QueueGroupRequiredState() {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-record-empty data-queue-empty aria-labelledby="queue-group-required-title">
      <h2 id="queue-group-required-title" data-state-heading>
        <Icon name="usersRound" />
        {t("queue.groupRequired.title")}
      </h2>
      <p>{t("queue.groupRequired.description")}</p>
      <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
        <Icon name="usersRound" />
        {t("queue.groupRequired.select")}
      </Link>
    </section>
  );
}

function QueueNoGroupsState() {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-record-empty data-queue-empty aria-labelledby="queue-no-groups-title">
      <h2 id="queue-no-groups-title" data-state-heading>
        <Icon name="usersRound" />
        {t("queue.noGroups.title")}
      </h2>
      <p>{t("queue.noGroups.description")}</p>
    </section>
  );
}

function QueueUnavailableState({
  error,
  onRetry
}: Readonly<{ error: ApiRequestError; onRetry: () => void }>) {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-record-empty data-queue-empty data-queue-unavailable role="alert" aria-labelledby="queue-unavailable-title">
      <h2 id="queue-unavailable-title" data-state-heading>
        <Icon name="circleAlert" />
        {t("queue.unavailable.title")}
      </h2>
      <p>{t(queueErrorMessageKey(error, "queue.errors.loadUnavailable"))}</p>
      <button type="button" data-slot="button" data-variant="outline" data-size="sm" onClick={onRetry}>
        <Icon name="refreshCw" />
        {t("queue.unavailable.retry")}
      </button>
    </section>
  );
}

type QueueFilteredEmptyStateProps = Readonly<{
  filter: QueueFilter;
  onClear: () => void;
}>;

function QueueFilteredEmptyState({ filter, onClear }: QueueFilteredEmptyStateProps) {
  const { t } = useTranslation();
  const group = filter.groupLabelKey ? t(filter.groupLabelKey) : filter.groupKey;

  return (
    <section data-slot="card" data-record-empty data-queue-empty aria-labelledby="queue-filtered-empty-title">
      <h2 id="queue-filtered-empty-title" data-state-heading>
        <Icon name="inbox" />
        {t("queue.filteredEmpty.title")}
      </h2>
      <p>
        {t("queue.filteredEmpty.currentCondition", {
          group,
          result: t(filter.result.labelKey)
        })}
      </p>
      <div>
        <button type="button" data-slot="button" data-variant="ghost" data-size="sm" onClick={onClear}>
          <Icon name="listX" />
          {t("queue.filteredEmpty.clear")}
        </button>
      </div>
    </section>
  );
}

export function QueueScreen() {
  const { i18n, t } = useTranslation();
  const session = useConsoleSession();
  const [searchParams, setSearchParams] = useSearchParams();
  const fixture = queueFixtureFor(searchParams.get("fixture"));
  const selectedGroupId = searchParams.get("group");
  const chatID =
    selectedGroupId !== null && /^-?\d+$/.test(selectedGroupId) ? selectedGroupId : undefined;
  const [queueState, setQueueState] = useState<QueueScreenState>({ kind: "loading" });
  const [records, setRecords] = useState<readonly QueueRecord[]>([]);
  const [pendingActions, setPendingActions] = useState<PendingQueueActions>({});
  const [feedback, setFeedback] = useState<QueueFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const inFlightRecordIdsRef = useRef(new Set<string>());
  const activeScopeRef = useRef("");
  const feedbackSequenceRef = useRef(0);
  const feedbackTimerRef = useRef<number | undefined>(undefined);
  const fixtureTimerIdsRef = useRef(new Set<number>());
  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat(i18n.resolvedLanguage ?? i18n.language, {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hourCycle: "h23"
      }),
    [i18n.language, i18n.resolvedLanguage]
  );

  useEffect(() => {
    const scope = `${session.state}:${chatID ?? ""}`;
    activeScopeRef.current = scope;
    let active = true;

    setRecords([]);

    if (session.state === "loading" || session.state === "checking-groups") {
      setQueueState({ kind: "loading" });
      return () => {
        active = false;
      };
    }

    if (session.state === "blocked" && session.error.kind === "non-json") {
      setRecords([...fixture.records]);
      setQueueState({ kind: "fixture", fixture });
      return () => {
        active = false;
      };
    }

    if (session.state === "blocked" || session.state === "groups-unavailable") {
      setQueueState({ kind: "unavailable", error: session.error });
      return () => {
        active = false;
      };
    }

    if (session.state === "no-groups") {
      setQueueState({ kind: "no-groups" });
      return () => {
        active = false;
      };
    }

    if (!chatID) {
      setQueueState({ kind: "group-required" });
      return () => {
        active = false;
      };
    }

    setQueueState({ kind: "loading" });
    void loadQueue(consoleApi, chatID).then((result) => {
      if (!active || activeScopeRef.current !== scope) {
        return;
      }

      if (result.ok) {
        setRecords(result.data);
        setQueueState({ kind: "loaded" });
        return;
      }

      setQueueState({ kind: "unavailable", error: result.error });
    });

    return () => {
      active = false;
    };
  }, [chatID, fixture, reloadVersion, session]);

  useEffect(() => {
    if (feedbackTimerRef.current !== undefined) {
      window.clearTimeout(feedbackTimerRef.current);
      feedbackTimerRef.current = undefined;
    }
    fixtureTimerIdsRef.current.forEach((timerID) => window.clearTimeout(timerID));
    fixtureTimerIdsRef.current.clear();
    inFlightRecordIdsRef.current.clear();
    setPendingActions({});
    setFeedback(null);

    return () => {
      if (feedbackTimerRef.current !== undefined) {
        window.clearTimeout(feedbackTimerRef.current);
        feedbackTimerRef.current = undefined;
      }
      fixtureTimerIdsRef.current.forEach((timerID) => window.clearTimeout(timerID));
      fixtureTimerIdsRef.current.clear();
      inFlightRecordIdsRef.current.clear();
    };
  }, [chatID, session.state]);

  function showFeedback(
    messageKey: string,
    tone: StatusTone,
    record: QueueRecord,
    dismissAfter: number | undefined
  ): void {
    if (feedbackTimerRef.current !== undefined) {
      window.clearTimeout(feedbackTimerRef.current);
      feedbackTimerRef.current = undefined;
    }

    const id = ++feedbackSequenceRef.current;
    setFeedback({ id, messageKey, tone, record });

    if (dismissAfter !== undefined) {
      const timerID = window.setTimeout(() => {
        if (feedbackSequenceRef.current === id) {
          setFeedback(null);
        }
        feedbackTimerRef.current = undefined;
      }, dismissAfter);
      feedbackTimerRef.current = timerID;
    }
  }

  function finishAction(recordID: string, scope: string): void {
    if (activeScopeRef.current !== scope) {
      return;
    }

    inFlightRecordIdsRef.current.delete(recordID);
    setPendingActions((currentActions) => {
      const nextActions = { ...currentActions };
      delete nextActions[recordID];
      return nextActions;
    });
  }

  function releaseFixtureRecord(record: QueueRecord, scope: string, fixtureData: QueueFixture): void {
    const shouldFail =
      fixtureData.records.find((fixtureRecord) => fixtureRecord.id === record.id)
        ?.simulatedFailureAction === "release";
    setRecords((currentRecords) =>
      currentRecords.map((currentRecord) =>
        currentRecord.id === record.id
          ? { ...currentRecord, result: challengeResults.approved, remainingSeconds: undefined }
          : currentRecord
      )
    );
    const timerID = window.setTimeout(() => {
      fixtureTimerIdsRef.current.delete(timerID);
      if (activeScopeRef.current !== scope) {
        return;
      }

      if (shouldFail) {
        setRecords((currentRecords) =>
          currentRecords.map((currentRecord) =>
            currentRecord.id === record.id ? record : currentRecord
          )
        );
        showFeedback("queue.feedback.releaseFailure", "error", record, undefined);
      } else {
        showFeedback("queue.feedback.releaseSuccess", "ok", record, FEEDBACK_DURATION_MS);
      }
      finishAction(record.id, scope);
    }, FIXTURE_ACTION_DELAY_MS);
    fixtureTimerIdsRef.current.add(timerID);
  }

  function releaseRecord(record: QueueRecord): void {
    if (inFlightRecordIdsRef.current.has(record.id)) {
      return;
    }

    const scope = activeScopeRef.current;
    inFlightRecordIdsRef.current.add(record.id);
    setPendingActions((currentActions) => ({ ...currentActions, [record.id]: true }));
    setFeedback(null);

    if (queueState.kind === "fixture") {
      releaseFixtureRecord(record, scope, queueState.fixture);
      return;
    }

    if (queueState.kind !== "loaded" || !chatID) {
      finishAction(record.id, scope);
      return;
    }

    void releaseQueueRecord(consoleApi, chatID, record)
      .then((result) => {
        if (activeScopeRef.current !== scope) {
          return;
        }

        if (result.ok) {
          setRecords((currentRecords) =>
            currentRecords.map((currentRecord) =>
              currentRecord.id === record.id ? result.data : currentRecord
            )
          );
          showFeedback("queue.feedback.releaseSuccess", "ok", result.data, FEEDBACK_DURATION_MS);
          return;
        }

        const error = result.error;
        showFeedback(
          queueErrorMessageKey(error, "queue.errors.settlementUnavailable"),
          "error",
          record,
          undefined
        );

        if (error.kind === "api" && error.code === "challenge_conflict") {
          setReloadVersion((currentVersion) => currentVersion + 1);
        }

        if (error.kind === "api" && accessRevocationCodes[error.code] === true) {
          setRecords([]);
          setQueueState({ kind: "unavailable", error });
        }
      })
      .finally(() => {
        finishAction(record.id, scope);
      });
  }

  function clearFixture(): void {
    setSearchParams((currentSearchParams) => {
      const nextSearchParams = new URLSearchParams(currentSearchParams);
      nextSearchParams.delete("fixture");

      return nextSearchParams;
    });
  }

  const dataState =
    queueState.kind === "fixture"
      ? queueState.fixture.id
      : queueState.kind === "loaded"
        ? records.length === 0
          ? "empty"
          : "populated"
        : queueState.kind;

  return (
    <section
      data-record-page
      data-queue-page
      data-queue-state={dataState}
      aria-busy={queueState.kind === "loading" ? true : undefined}
      aria-labelledby="queue-title"
    >
      <header data-page-heading>
        <h1 id="queue-title">
          <Icon name="inbox" />
          {t("queue.title")}
        </h1>
      </header>

      {queueState.kind === "loading" ? <QueueLoadingState /> : null}
      {queueState.kind === "group-required" ? <QueueGroupRequiredState /> : null}
      {queueState.kind === "no-groups" ? <QueueNoGroupsState /> : null}
      {queueState.kind === "unavailable" ? (
        <QueueUnavailableState
          error={queueState.error}
          onRetry={() => setReloadVersion((currentVersion) => currentVersion + 1)}
        />
      ) : null}
      {(queueState.kind === "fixture" || queueState.kind === "loaded") && records.length > 0 ? (
        <QueueTable
          records={records}
          pendingActions={pendingActions}
          dateFormatter={dateFormatter}
          onRelease={releaseRecord}
        />
      ) : null}
      {queueState.kind === "fixture" && records.length === 0 && queueState.fixture.filter ? (
        <QueueFilteredEmptyState filter={queueState.fixture.filter} onClear={clearFixture} />
      ) : null}
      {(queueState.kind === "fixture" || queueState.kind === "loaded") && records.length === 0 &&
      !(queueState.kind === "fixture" && queueState.fixture.filter) ? (
        <QueueEmptyState />
      ) : null}

      {feedback ? <QueueFeedbackNotice feedback={feedback} /> : null}
    </section>
  );
}
