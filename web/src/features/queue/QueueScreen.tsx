import { Button } from "@react-spectrum/s2/Button";
import { ButtonGroup } from "@react-spectrum/s2/ButtonGroup";
import { Card } from "@react-spectrum/s2/Card";
import { Content, Header, Heading, Text } from "@react-spectrum/s2";
import { InlineAlert } from "@react-spectrum/s2/InlineAlert";
import { TextField } from "@react-spectrum/s2/TextField";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import { consoleApi, retryConsoleAccess, useConsoleSession } from "../../app/session";
import { useConsoleSize } from "../../components/ConsoleProvider";
import type { StatusTone } from "../../components/StatusBadge";
import type { ApiRequestError } from "../../lib/api";
import { Icon } from "../../icons";
import { challengeResults } from "../../lib/challenge";
import { groupName } from "../../lib/chatNames";
import { loadQueue, releaseQueueRecord, type QueueRecord } from "./api";
import { queueFixtureFor, type QueueFixture } from "./fixtures";
import {
  QueueEmptyState,
  QueueFilteredEmptyState,
  QueueGroupRequiredState,
  QueueLoadingState,
  QueueNoGroupsState,
  QueueUnavailableState
} from "./QueueState";
import { QueueTable, type PendingQueueActions } from "./QueueTable";
const queuePageStyles = style({
  display: "grid",
  minWidth: 0,
  gap: 24
});

const queueHeadingStyles = style({
  margin: 0,
  display: "flex",
  alignItems: "center",
  gap: 8
});

const queueResultsStyles = style({
  display: "grid",
  minWidth: 0,
  gap: 24,
  minHeight: 192
});

const queueToolbarStyles = style({
  display: "flex",
  alignItems: "end",
  justifyContent: "space-between",
  flexWrap: "wrap",
  gap: 24
});

const queueTableContainerStyles = style({
  width: "full",
  minWidth: 0,
  overflowX: "auto"
});

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

function QueueFeedbackNotice({
  feedback,
  onReload
}: Readonly<{ feedback: QueueFeedback; onReload: () => void }>) {
  const { t } = useTranslation();
  // A dropped connection and a settlement the server rejected as stale both leave the
  // screen showing something the operator cannot act on. Naming the recovery is not the
  // same as offering it, so the notice carries the reload it names.
  const reloadable =
    feedback.messageKey === "queue.errors.network" ||
    feedback.messageKey === "queue.errors.invalidSettlement";
  const session = useConsoleSession();
  const group = groupName(
    feedback.record.groupKey,
    feedback.record.groupLabelKey
      ? t(feedback.record.groupLabelKey)
      : session.state === "ready" ? session.chats.find((chat) => chat.id === feedback.record.groupKey)?.title : undefined
  );
  const variant =
    feedback.tone === "ok"
      ? "positive"
      : feedback.tone === "error"
        ? "negative"
        : feedback.tone === "pending"
          ? "notice"
          : feedback.tone === "info"
            ? "informative"
            : "neutral";

  return (
    <InlineAlert
      data-record-feedback
      data-queue-feedback
      data-tone={feedback.tone}
      aria-atomic="true"
      variant={variant}
    >
      <Text>
        {t(feedback.messageKey, {
          user: feedback.record.user,
          group,
          approved: t(challengeResults.approved.labelKey)
        })}
      </Text>
      {reloadable ? (
        <ButtonGroup>
          <Button variant="secondary" onPress={onReload}>
            {t("queue.actions.reload")}
          </Button>
        </ButtonGroup>
      ) : null}
    </InlineAlert>
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
  const filterQuery = searchParams.get("q") ?? "";
  const [pendingActions, setPendingActions] = useState<PendingQueueActions>({});
  const [feedback, setFeedback] = useState<QueueFeedback | null>(null);

  function reloadQueue(): void {
    setFeedback(null);
    if (!retryConsoleAccess(session)) {
      setReloadVersion((currentVersion) => currentVersion + 1);
    }
  }
  const [reloadVersion, setReloadVersion] = useState(0);
  const filterSize = useConsoleSize("L");
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
  function setFilterQuery(query: string): void {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (query) next.set("q", query);
      else next.delete("q");
      return next;
    }, { replace: true });
  }

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

  const normalizedQuery = filterQuery.trim().toLocaleLowerCase();
  const visibleRecords = useMemo(
    () =>
      normalizedQuery.length === 0
        ? records
        : records.filter((record) => {
            const group = groupName(
              record.groupKey,
              record.groupLabelKey
                ? t(record.groupLabelKey)
                : session.state === "ready" ? session.chats.find((chat) => chat.id === record.groupKey)?.title : undefined
            );
            return [record.user, record.groupKey, group].some((value) =>
              value.toLocaleLowerCase().includes(normalizedQuery)
            );
          }),
    [normalizedQuery, records, session, t]
  );
  const isDataReady = queueState.kind === "fixture" || queueState.kind === "loaded";
  const hasRecords = isDataReady && records.length > 0;
  const tableEmptyState =
    queueState.kind === "fixture" && records.length === 0 && queueState.fixture.filter ? (
      <QueueFilteredEmptyState filter={queueState.fixture.filter} onClear={clearFixture} />
    ) : hasRecords && visibleRecords.length === 0 ? (
      <QueueFilteredEmptyState query={filterQuery} onClear={() => setFilterQuery("")} />
    ) : (
      <QueueEmptyState />
    );
  const dataState =
    queueState.kind === "fixture"
      ? queueState.fixture.id
      : queueState.kind === "loaded"
        ? records.length === 0
          ? "empty"
          : "populated"
        : queueState.kind;

  return (
    <Content
      data-record-page
      data-queue-page
      data-console-page
      data-queue-state={dataState}
      aria-busy={queueState.kind === "loading" ? true : undefined}
      aria-labelledby="queue-title"
      styles={queuePageStyles}
    >
      <Header data-page-heading>
        <Heading id="queue-title" level={1} styles={queueHeadingStyles}>
          <Icon name="inbox" />
          <Text>{t("queue.title")}</Text>
        </Heading>
      </Header>

      <Content data-queue-results styles={queueResultsStyles}>
        {hasRecords ? (
          <Card data-queue-toolbar density="compact" styles={style({ width: "full", minWidth: 0 })}>
            <Content styles={queueToolbarStyles}>
              <TextField
                label={t("queue.filter.label")}
                placeholder={t("queue.filter.placeholder")}
                value={filterQuery}
                onChange={setFilterQuery}
                size={filterSize}
                data-console-control
                data-control-size={filterSize}
                data-queue-filter
              />
              {filterQuery.length > 0 ? (
                <ButtonGroup>
                  <Button
                    variant="secondary"
                    fillStyle="outline"
                    size={filterSize}
                    data-console-control
                    data-control-size={filterSize}
                    data-queue-filter-clear
                    onPress={() => setFilterQuery("")}
                  >
                    <Text>{t("queue.filteredEmpty.clear")}</Text>
                  </Button>
                </ButtonGroup>
              ) : null}
            </Content>
          </Card>
        ) : null}

        {queueState.kind === "loading" ? <QueueLoadingState /> : null}
        {queueState.kind === "group-required" ? <QueueGroupRequiredState /> : null}
        {queueState.kind === "no-groups" ? <QueueNoGroupsState /> : null}
        {queueState.kind === "unavailable" ? (
          <QueueUnavailableState
            messageKey={queueErrorMessageKey(queueState.error, "queue.errors.loadUnavailable")}
            onRetry={reloadQueue}
          />
        ) : null}
        {isDataReady ? (
          <Content data-queue-table-container styles={queueTableContainerStyles}>
            <QueueTable
              key={chatID ?? "unselected"}
              records={visibleRecords}
              pendingActions={pendingActions}
              dateFormatter={dateFormatter}
              emptyState={tableEmptyState}
              onRelease={releaseRecord}
            />
          </Content>
        ) : null}
      </Content>

      {feedback ? <QueueFeedbackNotice feedback={feedback} onReload={reloadQueue} /> : null}
    </Content>
  );
}
