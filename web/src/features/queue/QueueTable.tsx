import {
  Cell,
  Column,
  Row,
  TableBody,
  TableHeader,
  TableView
} from "@react-spectrum/s2/TableView";
import { Button } from "@react-spectrum/s2/Button";
import { Content } from "@react-spectrum/s2/Content";
import { StatusLight } from "@react-spectrum/s2/StatusLight";
import { Text } from "@react-spectrum/s2/Text";
import { style } from "@react-spectrum/s2/style" with { type: "macro" };
import type { SortDescriptor } from "@react-spectrum/s2";
import { useMemo, useState, type ReactElement } from "react";
import { useTranslation } from "react-i18next";

import { useConsoleSession } from "../../app/session";
import { useConsoleSize } from "../../components/ConsoleProvider";
import type { StatusTone } from "../../components/StatusBadge";
import { Icon } from "../../icons";
import { groupName } from "../../lib/chatNames";
import type { QueueRecord } from "./api";

export type PendingQueueActions = Readonly<Record<string, true>>;

type QueueTableProps = Readonly<{
  records: readonly QueueRecord[];
  pendingActions: PendingQueueActions;
  dateFormatter: Intl.DateTimeFormat;
  emptyState: ReactElement;
  onRelease: (record: QueueRecord) => void;
}>;

type QueueRow = Readonly<{
  record: QueueRecord;
  pending: boolean;
  remainingTime: string | null;
  group: string;
  occurredAt: string;
  occurredAtValue: number | null;
}>;

type QueueReleaseActionProps = Readonly<{
  record: QueueRecord;
  pending: boolean;
  onRelease: (record: QueueRecord) => void;
}>;

const tableStyles = style({
  width: "full",
  height: 320
});

function formatRemainingTime(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;

  return `${minutes}:${remainingSeconds.toString().padStart(2, "0")}`;
}

function statusVariant(tone: StatusTone): "informative" | "neutral" | "positive" | "notice" | "negative" {
  switch (tone) {
    case "ok":
      return "positive";
    case "error":
      return "negative";
    case "pending":
      return "notice";
    case "info":
      return "informative";
    default:
      return "neutral";
  }
}

function QueueResult({ record, remainingTime }: Readonly<{ record: QueueRecord; remainingTime: string | null }>) {
  const { t } = useTranslation();
  const label = remainingTime !== null
    ? t("queue.status.pending", { time: remainingTime })
    : t(record.result.labelKey);

  return <StatusLight variant={statusVariant(record.result.tone)}>{label}</StatusLight>;
}

function QueueReleaseAction({ record, pending, onRelease }: QueueReleaseActionProps) {
  const { t } = useTranslation();
  const size = useConsoleSize("M");

  if (!pending && record.result.state !== "pending") {
    return null;
  }

  return (
    <Button
      variant="accent"
      size={size}
      isPending={pending}
      aria-disabled={pending || undefined}
      data-console-control
      data-control-size={size}
      data-queue-action-id="release"
      onPress={() => onRelease(record)}
      aria-label={t(pending ? "queue.actions.releasingFor" : "queue.actions.releaseFor", {
        user: record.user
      })}
    >
      <Icon name="unlock" />
      <Text>{t(pending ? "queue.actions.releasing" : "queue.actions.release")}</Text>
    </Button>
  );
}

function compareRows(left: QueueRow, right: QueueRow, column: string, direction: number, collator: Intl.Collator): number {
  if (column === "occurredAt") {
    if (left.occurredAtValue === null && right.occurredAtValue !== null) return 1;
    if (left.occurredAtValue !== null && right.occurredAtValue === null) return -1;
    if (left.occurredAtValue === null && right.occurredAtValue === null) return 0;
    return (left.occurredAtValue! - right.occurredAtValue!) * direction;
  }

  const comparison =
    column === "group"
      ? collator.compare(left.group, right.group)
      : collator.compare(left.record.user, right.record.user);
  return comparison * direction;
}

export function QueueTable({ records, pendingActions, dateFormatter, emptyState, onRelease }: QueueTableProps) {
  const { t, i18n } = useTranslation();
  const session = useConsoleSession();
  const collator = useMemo(() => new Intl.Collator(i18n.language, { sensitivity: "base" }), [i18n.language]);
  const [sortDescriptor, setSortDescriptor] = useState<SortDescriptor>({
    column: "occurredAt",
    direction: "descending"
  });

  const rows = useMemo<readonly QueueRow[]>(
    () => records.map((record) => ({
      record,
      pending: pendingActions[record.id] === true,
      remainingTime:
        record.result.state === "pending" && record.remainingSeconds !== undefined
          ? formatRemainingTime(record.remainingSeconds)
          : null,
      group: groupName(
        record.groupKey,
        record.groupLabelKey
          ? t(record.groupLabelKey)
          : session.state === "ready" ? session.chats.find((chat) => chat.id === record.groupKey)?.title : undefined
      ),
      occurredAt: record.occurredAt
        ? dateFormatter.format(new Date(record.occurredAt))
        : t("queue.timeUnavailable"),
      occurredAtValue: record.occurredAt ? Date.parse(record.occurredAt) : null
    })),
    [dateFormatter, pendingActions, records, session, t]
  );

  const sortedRows = useMemo(() => {
    const direction = sortDescriptor.direction === "ascending" ? 1 : -1;
    return [...rows].sort((left, right) =>
      compareRows(left, right, String(sortDescriptor.column), direction, collator)
    );
  }, [collator, rows, sortDescriptor]);


  return (
    <TableView
      aria-label={t("queue.tableLabel")}
      selectionMode="multiple"
      styles={tableStyles}
      sortDescriptor={sortDescriptor}
      onSortChange={setSortDescriptor}
      density="regular"
      overflowMode="wrap"
      data-queue-table
    >
      <TableHeader>
        <Column id="user" isRowHeader minWidth={128} allowsSorting>{t("queue.columns.user")}</Column>
        <Column id="group" minWidth={160} allowsSorting>{t("queue.columns.group")}</Column>
        <Column id="result" minWidth={192}>{t("queue.columns.result")}</Column>
        <Column id="occurredAt" minWidth={192} allowsSorting>{t("queue.columns.time")}</Column>
        <Column id="actions" minWidth={224} align="end">{t("queue.columns.actions")}</Column>
      </TableHeader>
      <TableBody items={sortedRows} renderEmptyState={() => emptyState}>
        {(row) => (
          <Row
            id={row.record.id}
            textValue={`${row.record.user} ${row.group}`}
            data-queue-row={row.record.id}
            data-result={row.record.result.id}
            data-action-state={row.pending ? "pending" : "idle"}
          >
            <Cell textValue={row.record.user}>
              <Text data-queue-user>{row.record.user}</Text>
            </Cell>
            <Cell textValue={row.group}>
              <Text data-queue-group>{row.group}</Text>
            </Cell>
            <Cell>
              <Content data-queue-result>
                <QueueResult record={row.record} remainingTime={row.remainingTime} />
              </Content>
            </Cell>
            <Cell textValue={row.occurredAt}>
              <Text data-queue-time styles={style({ fontFamily: "code" })}>{row.occurredAt}</Text>
            </Cell>
            <Cell>
              <QueueReleaseAction record={row.record} pending={row.pending} onRelease={onRelease} />
            </Cell>
          </Row>
        )}
      </TableBody>
    </TableView>
  );
}
