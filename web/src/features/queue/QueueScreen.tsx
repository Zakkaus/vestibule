import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import { StatusBadge } from "../../components/StatusBadge";
import type { QueueFilter, QueueRecord } from "./fixtures";
import {
  queueActions,
  queueFixtureFor,
  queueResultPresentations
} from "./fixtures";

type QueueTableProps = {
  records: readonly QueueRecord[];
  dateFormatter: Intl.DateTimeFormat;
};

function formatRemainingTime(seconds: number): string {
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;

  return `${minutes}:${remainingSeconds.toString().padStart(2, "0")}`;
}

function QueueTable({ records, dateFormatter }: QueueTableProps) {
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
            const action = presentation.action ? queueActions[presentation.action] : null;
            const remainingTime =
              presentation.showsRemainingTime && record.remainingSeconds !== undefined
                ? formatRemainingTime(record.remainingSeconds)
                : null;

            return (
              <tr key={record.id}>
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
                  {action ? (
                    <button
                      type="button"
                      data-slot="button"
                      data-variant={action.variant}
                      data-size="sm"
                      aria-label={t(action.ariaLabelKey, { user: record.user })}
                    >
                      {t(action.labelKey)}
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

      {fixture.records.length > 0 ? (
        <QueueTable records={fixture.records} dateFormatter={dateFormatter} />
      ) : fixture.filter ? (
        <QueueFilteredEmptyState filter={fixture.filter} onClear={clearFixture} />
      ) : (
        <QueueEmptyState />
      )}
    </section>
  );
}
