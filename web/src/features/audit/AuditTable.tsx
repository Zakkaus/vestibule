import { useTranslation } from "react-i18next";

import { StatusBadge } from "../../components/StatusBadge";
import type { AuditRecord } from "./api";

export type PendingAuditActions = Readonly<Record<string, true>>;

type AuditTableProps = Readonly<{
  records: readonly AuditRecord[];
  pendingActions: PendingAuditActions;
  dateFormatter: Intl.DateTimeFormat;
  onUndo: (record: AuditRecord) => void;
}>;

type AuditActionProps = Readonly<{
  record: AuditRecord;
  pending: boolean;
  onUndo: (record: AuditRecord) => void;
}>;

function AuditAction({ record, pending, onUndo }: AuditActionProps) {
  const { t } = useTranslation();

  if (record.undoState === "available") {
    return (
      <button
        type="button"
        data-slot="button"
        data-variant="outline"
        data-size="sm"
        data-audit-action="undo"
        aria-disabled={pending ? true : undefined}
        aria-label={t(pending ? "audit.actions.undoingFor" : "audit.actions.undoFor", {
          user: record.user
        })}
        onClick={() => onUndo(record)}
      >
        {t(pending ? "audit.actions.undoing" : "audit.actions.undo")}
      </button>
    );
  }

  switch (record.undoState) {
    case "pending":
      return <StatusBadge tone="pending">{t("audit.undoState.pending")}</StatusBadge>;
    case "completed":
      return <StatusBadge tone="ok">{t("audit.undoState.completed")}</StatusBadge>;
    case "failed":
      return <StatusBadge tone="error">{t("audit.undoState.failed")}</StatusBadge>;
    default:
      return null;
  }
}

export function AuditTable({ records, pendingActions, dateFormatter, onUndo }: AuditTableProps) {
  const { t } = useTranslation();
  const rows = records.map((record) => ({
    record,
    pending: pendingActions[record.id] === true,
    group: record.groupLabelKey ? t(record.groupLabelKey) : record.groupKey,
    result: t(`challenge.state.${record.result.state}`),
    reason: record.result.state === "declined" ? t(record.result.labelKey) : "—",
    actor: record.settledBy ?? t("audit.actor.automatic"),
    settledAt: dateFormatter.format(new Date(record.settledAt))
  }));

  return (
    <>
      <div data-record-table-scroll data-audit-table-scroll>
        <table data-record-table data-audit-table aria-label={t("audit.tableLabel")}>
          <thead>
            <tr>
              <th scope="col">{t("audit.columns.user")}</th>
              <th scope="col">{t("audit.columns.group")}</th>
              <th scope="col">{t("audit.columns.result")}</th>
              <th scope="col">{t("audit.columns.reason")}</th>
              <th scope="col">{t("audit.columns.actor")}</th>
              <th scope="col">{t("audit.columns.time")}</th>
              <th scope="col" aria-label={t("audit.columns.actions")} />
            </tr>
          </thead>
          <tbody>
            {rows.map(({ record, pending, group, result, reason, actor, settledAt }) => (
              <tr
                key={record.id}
                data-audit-row={record.id}
                data-result={record.result.id}
                data-undo-state={pending ? "submitting" : record.undoState}
              >
                <td data-record-user>{record.user}</td>
                <td data-record-group>{group}</td>
                <td data-record-result>
                  <StatusBadge tone={record.result.tone}>{result}</StatusBadge>
                </td>
                <td data-record-reason>{reason}</td>
                <td data-record-actor>{actor}</td>
                <td data-record-time>{settledAt}</td>
                <td data-record-action>
                  <AuditAction record={record} pending={pending} onUndo={onUndo} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <ul data-record-card-list data-audit-card-list aria-label={t("audit.tableLabel")}>
        {rows.map(({ record, pending, group, result, reason, actor, settledAt }) => (
          <li
            key={record.id}
            data-slot="card"
            data-record-card-row={record.id}
            data-audit-card-row={record.id}
            data-result={record.result.id}
            data-undo-state={pending ? "submitting" : record.undoState}
          >
            <div data-record-card-header>
              <strong data-record-user>{record.user}</strong>
              <StatusBadge tone={record.result.tone}>{result}</StatusBadge>
            </div>
            <dl data-record-card-details>
              <div>
                <dt>{t("audit.columns.group")}</dt>
                <dd data-record-group>{group}</dd>
              </div>
              <div>
                <dt>{t("audit.columns.reason")}</dt>
                <dd data-record-reason>{reason}</dd>
              </div>
              <div>
                <dt>{t("audit.columns.actor")}</dt>
                <dd data-record-actor>{actor}</dd>
              </div>
              <div>
                <dt>{t("audit.columns.time")}</dt>
                <dd data-record-time>{settledAt}</dd>
              </div>
            </dl>
            <div data-record-card-action>
              <AuditAction record={record} pending={pending} onUndo={onUndo} />
            </div>
          </li>
        ))}
      </ul>
    </>
  );
}
