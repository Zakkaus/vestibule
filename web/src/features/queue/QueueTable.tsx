import { Button, Card, Group, Stack, Table, Text } from "@mantine/core";
import { useTranslation } from "react-i18next";

import { Icon } from "../../icons";
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
    <StatusBadge presentation="library" tone={record.result.tone}>
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
    <Button
      type="button"
      size="sm"
      variant="filled"
      data-queue-action-id="release"
      aria-disabled={pending || undefined}
      tabIndex={0}
      onClick={() => onRelease(record)}
      leftSection={<Icon name="unlock" />}
      aria-label={t(pending ? "queue.actions.releasingFor" : "queue.actions.releaseFor", {
        user: record.user
      })}
    >
      {t(pending ? "queue.actions.releasing" : "queue.actions.release")}
    </Button>
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
      <Card withBorder padding={0} visibleFrom="sm">
        <Table.ScrollContainer
          minWidth="48rem"
          type="native"
          data-queue-table-scroll
        >
          <Table data-queue-table aria-label={t("queue.tableLabel")} withRowBorders>
            <Table.Thead>
              <Table.Tr>
                <Table.Th scope="col">{t("queue.columns.user")}</Table.Th>
                <Table.Th scope="col">{t("queue.columns.group")}</Table.Th>
                <Table.Th scope="col">{t("queue.columns.result")}</Table.Th>
                <Table.Th scope="col">{t("queue.columns.time")}</Table.Th>
                <Table.Th scope="col" aria-label={t("queue.columns.actions")} />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {rows.map(({ record, pending, remainingTime, group, occurredAt }) => (
                <Table.Tr
                  key={record.id}
                  data-queue-row={record.id}
                  data-result={record.result.id}
                  data-action-state={pending ? "pending" : "idle"}
                >
                  <Table.Td data-queue-user>{record.user}</Table.Td>
                  <Table.Td data-queue-group>{group}</Table.Td>
                  <Table.Td data-queue-result>
                    <QueueResult record={record} remainingTime={remainingTime} />
                  </Table.Td>
                  <Table.Td data-queue-time>{occurredAt}</Table.Td>
                  <Table.Td data-queue-action>
                    <Group justify="flex-end" wrap="nowrap">
                      <QueueReleaseAction
                        record={record}
                        pending={pending}
                        onRelease={onRelease}
                      />
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      </Card>

      <Stack
        hiddenFrom="sm"
        gap="md"
        data-queue-card-list
        role="list"
        aria-label={t("queue.tableLabel")}
      >
        {rows.map(({ record, pending, remainingTime, group, occurredAt }) => (
          <Card
            component="div"
            key={record.id}
            withBorder
            padding="md"
            data-queue-card-row={record.id}
            role="listitem"
            data-result={record.result.id}
            data-action-state={pending ? "pending" : "idle"}
          >
            <Stack gap="sm">
              <Group
                justify="space-between"
                align="center"
                wrap="nowrap"
                gap="sm"
                data-queue-card-header
              >
                <Text fw={600} data-queue-user>
                  {record.user}
                </Text>
                <QueueResult record={record} remainingTime={remainingTime} />
              </Group>
              <Stack component="dl" gap="xs" data-queue-card-details>
                <Group component="div" justify="space-between" align="baseline" gap="md">
                  <Text component="dt" size="sm" c="dimmed">
                    {t("queue.columns.group")}
                  </Text>
                  <Text component="dd" size="sm" m={0} ta="end" data-queue-card-group>
                    {group}
                  </Text>
                </Group>
                <Group component="div" justify="space-between" align="baseline" gap="md">
                  <Text component="dt" size="sm" c="dimmed">
                    {t("queue.columns.time")}
                  </Text>
                  <Text component="dd" size="sm" m={0} ta="end" data-queue-time>
                    {occurredAt}
                  </Text>
                </Group>
              </Stack>
              <Group justify="flex-end" data-queue-card-action>
                <QueueReleaseAction record={record} pending={pending} onRelease={onRelease} />
              </Group>
            </Stack>
          </Card>
        ))}
      </Stack>
    </>
  );
}
