import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import { StatusBadge, type StatusTone } from "../../components/StatusBadge";
import { challengeResults } from "../../lib/challenge";
import type {
  QueueActionId,
  QueueFeedbackId,
  QueueFilter,
  QueueRecord
} from "./fixtures";
import {
  queueActions,
  queueFixtureFor,
  queueResultPresentations
} from "./fixtures";

const ACTION_DELAY_MS = 700;
const FEEDBACK_DURATION_MS = 5_000;

type PendingActions = Partial<Record<string, QueueActionId>>;

type QueueFeedback = {
  id: number;
  kind: QueueFeedbackId;
  record: QueueRecord;
};

type QueueConfirmation = {
  actionId: QueueActionId;
  record: QueueRecord;
};

type QueueTableProps = {
  records: readonly QueueRecord[];
  pendingActions: PendingActions;
  dateFormatter: Intl.DateTimeFormat;
  onAction: (record: QueueRecord, actionId: QueueActionId) => void;
};

function formatRemainingTime(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;

  return `${minutes}:${remainingSeconds.toString().padStart(2, "0")}`;
}

function QueueTable({
  records,
  pendingActions,
  dateFormatter,
  onAction
}: QueueTableProps) {
  const { t } = useTranslation();

  return (
    <div data-queue-table-scroll>
      <table data-queue-table aria-label={t("queue.tableLabel")}>
        <thead>
          <tr>
            <th scope="col">{t("queue.columns.user")}</th>
            <th scope="col">{t("queue.columns.group")}</th>
            <th scope="col">{t("queue.columns.result")}</th>
            <th scope="col">{t("queue.columns.time")}</th>
            <th scope="col" aria-label={t("queue.columns.actions")} />
          </tr>
        </thead>
        <tbody>
          {records.map((record) => {
            const presentation = queueResultPresentations[record.result.id];
            const pendingActionId = pendingActions[record.id];
            const actionId = pendingActionId ?? presentation.action;
            const action = actionId ? queueActions[actionId] : null;
            const remainingTime =
              presentation.showsRemainingTime && record.remainingSeconds !== undefined
                ? formatRemainingTime(record.remainingSeconds)
                : null;

            return (
              <tr
                key={record.id}
                data-queue-row={record.id}
                data-result={record.result.id}
                data-action-state={pendingActionId ? "pending" : "idle"}
              >
                <td data-queue-user>{record.user}</td>
                <td data-queue-group>{t(record.groupKey)}</td>
                <td data-queue-result>
                  <StatusBadge tone={record.result.tone}>
                    {remainingTime
                      ? t("queue.status.pending", { time: remainingTime })
                      : t(record.result.labelKey)}
                  </StatusBadge>
                </td>
                <td data-queue-time>{dateFormatter.format(new Date(record.occurredAt))}</td>
                <td data-queue-action>
                  {action && actionId ? (
                    <button
                      type="button"
                      data-slot="button"
                      data-variant={action.variant}
                      data-size="sm"
                      data-queue-action-id={actionId}
                      aria-disabled={pendingActionId ? true : undefined}
                      aria-label={t(
                        pendingActionId ? action.pendingAriaLabelKey : action.ariaLabelKey,
                        { user: record.user }
                      )}
                      onClick={() => onAction(record, actionId)}
                    >
                      {t(pendingActionId ? action.pendingLabelKey : action.labelKey)}
                    </button>
                  ) : null}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

const queueFeedbackPresentations: Readonly<
  Record<QueueFeedbackId, { messageKey: string; tone: StatusTone }>
> = {
  releaseSuccess: {
    messageKey: "queue.feedback.releaseSuccess",
    tone: "ok"
  },
  revokeSuccess: {
    messageKey: "queue.feedback.revokeSuccess",
    tone: "ok"
  },
  releaseFailure: {
    messageKey: "queue.feedback.releaseFailure",
    tone: "error"
  },
  detailsUnavailable: {
    messageKey: "queue.feedback.detailsUnavailable",
    tone: "info"
  }
};

function QueueFeedbackNotice({ feedback }: { feedback: QueueFeedback }) {
  const { t } = useTranslation();
  const presentation = queueFeedbackPresentations[feedback.kind];

  return (
    <div
      data-queue-feedback
      data-feedback-kind={feedback.kind}
      data-tone={presentation.tone}
      role={presentation.tone === "error" ? "alert" : "status"}
      aria-atomic="true"
    >
      {t(presentation.messageKey, {
        user: feedback.record.user,
        group: t(feedback.record.groupKey),
        approved: t(challengeResults.approved.labelKey),
        pending: t(challengeResults.pending.labelKey)
      })}
    </div>
  );
}

type QueueConfirmationDialogProps = {
  confirmation: QueueConfirmation | null;
  onCancel: () => void;
  onConfirm: (confirmation: QueueConfirmation) => void;
};

function QueueConfirmationDialog({
  confirmation,
  onCancel,
  onConfirm
}: QueueConfirmationDialogProps) {
  const { t } = useTranslation();
  const dialogRef = useRef<HTMLDialogElement>(null);
  const action = confirmation ? queueActions[confirmation.actionId] : null;
  const copy = action?.confirmation ?? null;

  useEffect(() => {
    const dialog = dialogRef.current;

    if (!dialog) {
      return;
    }

    if (confirmation && copy && !dialog.open) {
      dialog.showModal();
    } else if ((!confirmation || !copy) && dialog.open) {
      dialog.close();
    }
  }, [confirmation, copy]);

  function cancel(): void {
    if (dialogRef.current?.open) {
      dialogRef.current.close();
    }

    onCancel();
  }

  function confirm(): void {
    if (!confirmation) {
      return;
    }

    if (dialogRef.current?.open) {
      dialogRef.current.close();
    }

    onConfirm(confirmation);
  }

  return (
    <dialog
      ref={dialogRef}
      data-queue-confirmation
      aria-labelledby="queue-confirmation-title"
      aria-describedby="queue-confirmation-description"
      onCancel={(event) => {
        event.preventDefault();
        cancel();
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) {
          cancel();
        }
      }}
    >
      {confirmation && copy ? (
        <div data-slot="card" data-queue-dialog-content>
          <h2 id="queue-confirmation-title">
            {t(copy.titleKey, { user: confirmation.record.user })}
          </h2>
          <p id="queue-confirmation-description">
            {t(copy.descriptionKey, {
              user: confirmation.record.user,
              group: t(confirmation.record.groupKey),
              approved: t(challengeResults.approved.labelKey)
            })}
          </p>
          <div data-queue-dialog-actions>
            <button
              type="button"
              data-slot="button"
              data-variant="ghost"
              data-size="sm"
              onClick={cancel}
            >
              {t(copy.cancelKey)}
            </button>
            <button
              type="button"
              data-slot="button"
              data-variant="destructive"
              data-emphasis="solid"
              data-size="sm"
              onClick={confirm}
            >
              {t(copy.confirmKey)}
            </button>
          </div>
        </div>
      ) : null}
    </dialog>
  );
}

function QueueEmptyState() {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-queue-empty aria-labelledby="queue-empty-title">
      <h2 id="queue-empty-title">{t("queue.empty.title")}</h2>
      <p>{t("queue.empty.description")}</p>
    </section>
  );
}

type QueueFilteredEmptyStateProps = {
  filter: QueueFilter;
  onClear: () => void;
};

function QueueFilteredEmptyState({ filter, onClear }: QueueFilteredEmptyStateProps) {
  const { t } = useTranslation();

  return (
    <section data-slot="card" data-queue-empty aria-labelledby="queue-filtered-empty-title">
      <h2 id="queue-filtered-empty-title">{t("queue.filteredEmpty.title")}</h2>
      <p>
        {t("queue.filteredEmpty.currentCondition", {
          group: t(filter.groupKey),
          result: t(filter.result.labelKey)
        })}
      </p>
      <div>
        <button type="button" data-slot="button" data-variant="ghost" data-size="sm" onClick={onClear}>
          {t("queue.filteredEmpty.clear")}
        </button>
      </div>
    </section>
  );
}

export function QueueScreen() {
  const { i18n, t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const fixture = queueFixtureFor(searchParams.get("fixture"));
  const [records, setRecords] = useState<QueueRecord[]>(() => [...fixture.records]);
  const [pendingActions, setPendingActions] = useState<PendingActions>({});
  const [confirmation, setConfirmation] = useState<QueueConfirmation | null>(null);
  const [feedback, setFeedback] = useState<QueueFeedback[]>([]);
  const inFlightRecordIdsRef = useRef(new Set<string>());
  const actionTimerIdsRef = useRef(new Set<number>());
  const feedbackTimerIdsRef = useRef(new Set<number>());
  const feedbackSequenceRef = useRef(0);
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
    actionTimerIdsRef.current.forEach((timerId) => window.clearTimeout(timerId));
    feedbackTimerIdsRef.current.forEach((timerId) => window.clearTimeout(timerId));
    actionTimerIdsRef.current.clear();
    feedbackTimerIdsRef.current.clear();
    inFlightRecordIdsRef.current.clear();
    setRecords([...fixture.records]);
    setPendingActions({});
    setConfirmation(null);
    setFeedback([]);

    return () => {
      actionTimerIdsRef.current.forEach((timerId) => window.clearTimeout(timerId));
      feedbackTimerIdsRef.current.forEach((timerId) => window.clearTimeout(timerId));
      actionTimerIdsRef.current.clear();
      feedbackTimerIdsRef.current.clear();
      inFlightRecordIdsRef.current.clear();
    };
  }, [fixture]);

  function showFeedback(kind: QueueFeedbackId, record: QueueRecord): void {
    const feedbackId = ++feedbackSequenceRef.current;
    setFeedback((currentFeedback) => [
      ...currentFeedback,
      { id: feedbackId, kind, record }
    ]);

    const timerId = window.setTimeout(() => {
      feedbackTimerIdsRef.current.delete(timerId);
      setFeedback((currentFeedback) =>
        currentFeedback.filter((item) => item.id !== feedbackId)
      );
    }, FEEDBACK_DURATION_MS);
    feedbackTimerIdsRef.current.add(timerId);
  }

  function startAction(record: QueueRecord, actionId: QueueActionId): void {
    if (inFlightRecordIdsRef.current.has(record.id)) {
      return;
    }

    const action = queueActions[actionId];
    const shouldFail = record.simulatedFailureAction === actionId;

    if (shouldFail && action.failureFeedback === null) {
      throw new Error(`Queue fixture cannot fail the ${actionId} action`);
    }

    inFlightRecordIdsRef.current.add(record.id);
    setPendingActions((currentActions) => ({
      ...currentActions,
      [record.id]: actionId
    }));

    const optimisticResult = action.optimisticResult;
    if (optimisticResult) {
      setRecords((currentRecords) =>
        currentRecords.map((currentRecord) =>
          currentRecord.id === record.id
            ? {
                ...currentRecord,
                result: optimisticResult,
                remainingSeconds: undefined
              }
            : currentRecord
        )
      );
    }

    const timerId = window.setTimeout(() => {
      actionTimerIdsRef.current.delete(timerId);

      if (shouldFail) {
        setRecords((currentRecords) =>
          currentRecords.map((currentRecord) =>
            currentRecord.id === record.id ? record : currentRecord
          )
        );
      }

      setPendingActions((currentActions) => {
        const nextActions = { ...currentActions };
        delete nextActions[record.id];
        return nextActions;
      });
      inFlightRecordIdsRef.current.delete(record.id);
      showFeedback(
        shouldFail ? action.failureFeedback! : action.completionFeedback,
        record
      );
    }, ACTION_DELAY_MS);
    actionTimerIdsRef.current.add(timerId);
  }

  function handleAction(record: QueueRecord, actionId: QueueActionId): void {
    if (inFlightRecordIdsRef.current.has(record.id)) {
      return;
    }

    if (queueActions[actionId].confirmation) {
      setConfirmation({ actionId, record });
      return;
    }

    startAction(record, actionId);
  }

  function clearFixture(): void {
    setSearchParams((currentSearchParams) => {
      const nextSearchParams = new URLSearchParams(currentSearchParams);
      nextSearchParams.delete("fixture");

      return nextSearchParams;
    });
  }

  return (
    <section data-queue-page data-queue-state={fixture.id} aria-labelledby="queue-title">
      <header data-page-heading>
        <h1 id="queue-title">{t("queue.title")}</h1>
      </header>

      {records.length > 0 ? (
        <QueueTable
          records={records}
          pendingActions={pendingActions}
          dateFormatter={dateFormatter}
          onAction={handleAction}
        />
      ) : fixture.filter ? (
        <QueueFilteredEmptyState filter={fixture.filter} onClear={clearFixture} />
      ) : (
        <QueueEmptyState />
      )}

      {feedback.length > 0 ? (
        <div data-queue-feedback-stack>
          {feedback.map((item) => (
            <QueueFeedbackNotice key={item.id} feedback={item} />
          ))}
        </div>
      ) : null}

      <QueueConfirmationDialog
        confirmation={confirmation}
        onCancel={() => setConfirmation(null)}
        onConfirm={(confirmedAction) => {
          setConfirmation(null);
          startAction(confirmedAction.record, confirmedAction.actionId);
        }}
      />
    </section>
  );
}
