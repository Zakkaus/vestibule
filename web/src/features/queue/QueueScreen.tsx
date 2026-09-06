import type { ReactNode } from "react";
import { Alert, Button, Group, Paper, Stack, Text, Title } from "@mantine/core";
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import { consoleApi, useConsoleSession } from "../../app/session";
import type { StatusTone } from "../../components/StatusBadge";
import type { ApiRequestError } from "../../lib/api";
import { Icon, type IconName } from "../../icons";
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

const feedbackColorByTone: Readonly<Record<StatusTone, string>> = {
  ok: "teal",
  info: "blue",
  pending: "yellow",
  error: "red",
  neutral: "gray"
};

const feedbackIconByTone: Readonly<Record<StatusTone, IconName>> = {
  ok: "circleCheck",
  info: "info",
  pending: "loaderCircle",
  error: "circleAlert",
  neutral: "circleMinus"
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

type QueueStateCardProps = Readonly<{
  id: string;
  icon: IconName;
  descriptionKey?: string;
  titleKey: string;
  role?: "alert";
  live?: "polite";
  dataQueueUnavailable?: boolean;
  children?: ReactNode;
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
function QueueStateCard({
  id,
  icon,
  titleKey,
  descriptionKey,
  role,
  live,
  dataQueueUnavailable,
  children
}: QueueStateCardProps) {
  const { t } = useTranslation();

  return (
    <Paper
      withBorder
      p="lg"
      data-queue-empty
      data-queue-state-card={id}
      data-queue-unavailable={dataQueueUnavailable ? true : undefined}
      role={role}
      aria-live={live}
      aria-labelledby={`queue-${id}-title`}
    >
      <Stack gap="sm">
        <Group gap="xs" align="center">
          <Icon name={icon} />
          <Title id={`queue-${id}-title`} order={2} size="h3">
            {t(titleKey)}
          </Title>
        </Group>
        {descriptionKey ? <Text>{t(descriptionKey)}</Text> : null}
        {children}
      </Stack>
    </Paper>
  );
}

function QueueFeedbackNotice({ feedback }: Readonly<{ feedback: QueueFeedback }>) {
  const { t } = useTranslation();
  const group = feedback.record.groupLabelKey
    ? t(feedback.record.groupLabelKey)
    : feedback.record.groupKey;

  return (
    <Alert
      data-queue-feedback
      data-tone={feedback.tone}
      role={feedback.tone === "error" ? "alert" : "status"}
      aria-atomic="true"
      color={feedbackColorByTone[feedback.tone]}
      variant="light"
      icon={<Icon name={feedbackIconByTone[feedback.tone]} />}
    >
      {t(feedback.messageKey, {
        user: feedback.record.user,
        group,
        approved: t(challengeResults.approved.labelKey)
      })}
    </Alert>
  );
}

function QueueEmptyState() {
  return (
    <QueueStateCard
      id="empty"
      icon="inbox"
      titleKey="queue.empty.title"
      descriptionKey="queue.empty.description"
    />
  );
}

function QueueLoadingState() {
  return (
    <QueueStateCard
      id="loading"
      icon="loaderCircle"
      titleKey="queue.loading.title"
      descriptionKey="queue.loading.description"
      live="polite"
    />
  );
}

function QueueGroupRequiredState() {
  const { t } = useTranslation();

  return (
    <QueueStateCard
      id="group-required"
      icon="usersRound"
      titleKey="queue.groupRequired.title"
      descriptionKey="queue.groupRequired.description"
    >
      <Button
        component={Link}
        to="/groups"
        size="sm"
        variant="filled"
        leftSection={<Icon name="usersRound" />}
      >
        {t("queue.groupRequired.select")}
      </Button>
    </QueueStateCard>
  );
}

function QueueNoGroupsState() {
  return (
    <QueueStateCard
      id="no-groups"
      icon="usersRound"
      titleKey="queue.noGroups.title"
      descriptionKey="queue.noGroups.description"
    />
  );
}

function QueueUnavailableState({
  error,
  onRetry
}: Readonly<{ error: ApiRequestError; onRetry: () => void }>) {
  const { t } = useTranslation();

  return (
    <QueueStateCard
      id="unavailable"
      icon="circleAlert"
      titleKey="queue.unavailable.title"
      descriptionKey={queueErrorMessageKey(error, "queue.errors.loadUnavailable")}
      role="alert"
      dataQueueUnavailable
    >
      <Button
        type="button"
        size="sm"
        variant="default"
        onClick={onRetry}
        leftSection={<Icon name="refreshCw" />}
      >
        {t("queue.unavailable.retry")}
      </Button>
    </QueueStateCard>
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
    <QueueStateCard
      id="filtered-empty"
      icon="inbox"
      titleKey="queue.filteredEmpty.title"
    >
      <Text>
        {t("queue.filteredEmpty.currentCondition", {
          group,
          result: t(filter.result.labelKey)
        })}
      </Text>
      <Button
        type="button"
        size="sm"
        variant="light"
        onClick={onClear}
        leftSection={<Icon name="listX" />}
      >
        {t("queue.filteredEmpty.clear")}
      </Button>
    </QueueStateCard>
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
    <Stack
      component="section"
      gap="lg"
      data-queue-page
      data-queue-state={dataState}
      aria-busy={queueState.kind === "loading" ? true : undefined}
      aria-labelledby="queue-title"
    >
      <Group
        component="header"
        justify="space-between"
        align="center"
        data-queue-toolbar
        data-queue-heading
      >
        <Group gap="xs" align="center">
          <Icon name="inbox" />
          <Title id="queue-title" order={1}>
            {t("queue.title")}
          </Title>
        </Group>
      </Group>

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
    </Stack>
  );
}
