import { useTranslation } from "react-i18next";

import { StatusBadge } from "../../components/StatusBadge";
import type { QueueRecord } from "./api";

export type PendingQueueActions = Readonly<Record<string, true>>;

type QueueTableProps = Readonly<{
  records: readonly QueueRecord[];
  pendingActions: PendingQueueActions;
  dateFormatter: Intl.DateTimeFormat;
  onRelease: (record: QueueRecord) => void;
}>;

type QueueResultProps = Readonly<{
  record: QueueRecord;
  remainingTime: string | null;
}>;

type QueueReleaseActionProps = Readonly<{
  record: QueueRecord;
  pending: boolean;
  onRelease: (record: QueueRecord) => void;
}>;

function formatRemainingTime(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;

  return `${minutes}:${remainingSeconds.toString().padStart(2, "0")}`;
}

function QueueResult({ record, remainingTime }: QueueResultProps) {
  const { t } = useTranslation();

  return (
    <StatusBadge tone={record.result.tone}>
      {remainingTime
        ? t("queue.status.pending", { time: remainingTime })
        : t(record.result.labelKey)}
    </StatusBadge>
  );
}

function QueueReleaseAction({ record, pending, onRelease }: QueueReleaseActionProps) {
  const { t } = useTranslation();

  if (record.result.state !== "pending") {
    return null;
  }

  return (
    <button
      type="button"
      data-slot="button"
      data-variant="primary"
      data-size="sm"
      data-queue-action-id="release"
      aria-disabled={pending ? true : undefined}
      aria-label={t(pending ? "queue.actions.releasingFor" : "queue.actions.releaseFor", {
        user: record.user
      })}
      onClick={() => onRelease(record)}
    >
      {t(pending ? "queue.actions.releasing" : "queue.actions.release")}
    </button>
  );
}

export function QueueTable({ records, pendingActions, dateFormatter, onRelease }: QueueTableProps) {
  const { t } = useTranslation();
  const rows = records.map((record) => {
    const pending = pendingActions[record.id] === true;
    const remainingTime =
      record.result.state === "pending" && record.remainingSeconds !== undefined
        ? formatRemainingTime(record.remainingSeconds)
        : null;

    return {
      record,
      pending,
      remainingTime,
      group: record.groupLabelKey ? t(record.groupLabelKey) : record.groupKey,
      occurredAt: record.occurredAt
        ? dateFormatter.format(new Date(record.occurredAt))
        : t("queue.timeUnavailable")
    };
  });

  return (
    <>
      <div data-record-table-scroll data-queue-table-scroll>
        <table data-record-table data-queue-table aria-label={t("queue.tableLabel")}>
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
            {rows.map(({ record, pending, remainingTime, group, occurredAt }) => (
              <tr
                key={record.id}
                data-queue-row={record.id}
                data-result={record.result.id}
                data-action-state={pending ? "pending" : "idle"}
              >
                <td data-record-user data-queue-user>{record.user}</td>
                <td data-record-group data-queue-group>{group}</td>
                <td data-record-result data-queue-result>
                  <QueueResult record={record} remainingTime={remainingTime} />
                </td>
                <td data-record-time data-queue-time>{occurredAt}</td>
                <td data-record-action data-queue-action>
                  <QueueReleaseAction record={record} pending={pending} onRelease={onRelease} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <ul data-record-card-list data-queue-card-list aria-label={t("queue.tableLabel")}>
        {rows.map(({ record, pending, remainingTime, group, occurredAt }) => (
          <li
            key={record.id}
            data-slot="card"
            data-record-card-row={record.id}
            data-queue-card-row={record.id}
            data-result={record.result.id}
            data-action-state={pending ? "pending" : "idle"}
          >
            <div data-record-card-header data-queue-card-header>
              <strong data-record-user data-queue-user>{record.user}</strong>
              <QueueResult record={record} remainingTime={remainingTime} />
            </div>
            <dl data-record-card-details data-queue-card-details>
              <div>
                <dt>{t("queue.columns.group")}</dt>
                <dd data-record-group data-queue-card-group>{group}</dd>
              </div>
              <div>
                <dt>{t("queue.columns.time")}</dt>
                <dd data-record-time data-queue-time>{occurredAt}</dd>
              </div>
            </dl>
            <div data-record-card-action data-queue-card-action>
              <QueueReleaseAction record={record} pending={pending} onRelease={onRelease} />
            </div>
          </li>
        ))}
      </ul>
    </>
  );
}
