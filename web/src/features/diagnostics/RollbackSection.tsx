import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { StatusBadge, type StatusTone } from "../../components/StatusBadge";
import type {
  DiagnosticsDatabaseWrites,
  DiagnosticsProblemStreak,
  DiagnosticsRejections,
  DiagnosticsRollback
} from "./api";
import { DetailRow, DiagnosticsCard } from "./DiagnosticsCard";
import {
  durationParts,
  rejectionsState,
  streakState,
  writeRateState,
  type DiagnosticsFormatters,
  type RejectionsState,
  type StreakState,
  type WriteRateState
} from "./model";

type ReadingBadge = Readonly<{ tone: StatusTone; labelKey: string }>;

const streakBadges: Readonly<Record<StreakState, ReadingBadge>> = {
  clear: { tone: "ok", labelKey: "diagnostics.values.streakClear" },
  within: { tone: "info", labelKey: "diagnostics.values.streakWithin" },
  exceeded: { tone: "error", labelKey: "diagnostics.values.streakExceeded" }
};

const writeRateBadges: Readonly<Record<WriteRateState, ReadingBadge>> = {
  "no-writes": { tone: "neutral", labelKey: "diagnostics.values.noWrites" },
  within: { tone: "ok", labelKey: "diagnostics.values.writeRateWithin" },
  exceeded: { tone: "error", labelKey: "diagnostics.values.writeRateExceeded" }
};

function useDurationText(): (totalSeconds: number) => string {
  const { t } = useTranslation();
  return (totalSeconds: number) => {
    const parts = durationParts(totalSeconds);
    switch (parts.unit) {
      case "seconds":
        return t("diagnostics.values.durationSeconds", { seconds: parts.seconds });
      case "minutes":
        return t("diagnostics.values.durationMinutes", { minutes: parts.minutes });
      case "hours":
        return t("diagnostics.values.durationHours", { hours: parts.hours });
      case "minutesSeconds":
        return t("diagnostics.values.durationMinutesSeconds", {
          minutes: parts.minutes,
          seconds: parts.seconds
        });
    }
  };
}

function ReadingItem({
  id,
  titleKey,
  state,
  badge,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  state: string;
  badge: ReadingBadge | null;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <li data-diagnostics-rollback-item={id} data-diagnostics-rollback-state={state}>
      <div data-diagnostics-rollback-heading>
        <h3>{t(titleKey)}</h3>
        {badge === null ? null : <StatusBadge tone={badge.tone}>{t(badge.labelKey)}</StatusBadge>}
      </div>
      {children}
    </li>
  );
}

function TimestampRow({
  name,
  labelKey,
  at,
  formatters
}: Readonly<{
  name: string;
  labelKey: string;
  at: string | null;
  formatters: DiagnosticsFormatters;
}>) {
  const { t } = useTranslation();
  return (
    <DetailRow name={name} labelKey={labelKey}>
      {at === null ? (
        <StatusBadge tone="neutral">{t("diagnostics.values.notRecorded")}</StatusBadge>
      ) : (
        <time dateTime={at}>{formatters.date.format(new Date(at))}</time>
      )}
    </DetailRow>
  );
}

function StreakReading({
  id,
  titleKey,
  streak,
  formatters,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  streak: DiagnosticsProblemStreak;
  formatters: DiagnosticsFormatters;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  const duration = useDurationText();
  const state = streakState(streak);
  return (
    <ReadingItem id={id} titleKey={titleKey} state={state} badge={streakBadges[state]}>
      <p data-diagnostics-rollback-reading>
        {t("diagnostics.rollback.streak.rule", {
          threshold: duration(streak.thresholdSeconds)
        })}
      </p>
      <dl data-diagnostics-values>
        <DetailRow name={`${id}-span`} labelKey="diagnostics.labels.problemSpan">
          <span data-diagnostics-metric="problem-span">
            {duration(streak.problemSpanSeconds)}
          </span>
        </DetailRow>
        <TimestampRow
          name={`${id}-first-problem`}
          labelKey="diagnostics.labels.firstProblem"
          at={streak.firstProblemAt}
          formatters={formatters}
        />
        <TimestampRow
          name={`${id}-last-problem`}
          labelKey="diagnostics.labels.lastProblem"
          at={streak.lastProblemAt}
          formatters={formatters}
        />
        <TimestampRow
          name={`${id}-last-recovered`}
          labelKey="diagnostics.labels.lastRecovered"
          at={streak.lastRecoveredAt}
          formatters={formatters}
        />
        {children}
      </dl>
    </ReadingItem>
  );
}

function CountRow({
  name,
  labelKey,
  value,
  formatters
}: Readonly<{
  name: string;
  labelKey: string;
  value: number;
  formatters: DiagnosticsFormatters;
}>) {
  return (
    <DetailRow name={name} labelKey={labelKey}>
      <span data-diagnostics-metric="count">{formatters.number.format(value)}</span>
    </DetailRow>
  );
}

function DatabaseWritesReading({
  writes,
  formatters
}: Readonly<{ writes: DiagnosticsDatabaseWrites; formatters: DiagnosticsFormatters }>) {
  const { t } = useTranslation();
  const duration = useDurationText();
  const state = writeRateState(writes);
  return (
    <ReadingItem
      id="database-writes"
      titleKey="diagnostics.labels.databaseWrites"
      state={state}
      badge={writeRateBadges[state]}
    >
      <p data-diagnostics-rollback-reading>
        {t("diagnostics.rollback.databaseWrites.rule", {
          window: duration(writes.windowSeconds)
        })}
      </p>
      <dl data-diagnostics-values>
        <DetailRow name="database-writes-rate" labelKey="diagnostics.labels.failureRate">
          {state === "no-writes" ? (
            <StatusBadge tone="neutral">{t("diagnostics.values.noWrites")}</StatusBadge>
          ) : (
            <span data-diagnostics-metric="failure-rate">
              {t("diagnostics.rollback.databaseWrites.rate", {
                rate: t("diagnostics.values.percent", {
                  value: formatters.rate.format(writes.failureRatePercent)
                }),
                failed: formatters.number.format(writes.failedWrites),
                total: formatters.number.format(writes.totalWrites)
              })}
            </span>
          )}
        </DetailRow>
        <DetailRow name="database-writes-scope" labelKey="diagnostics.labels.writeScope">
          <code>{writes.scope}</code>
        </DetailRow>
      </dl>
    </ReadingItem>
  );
}

function RejectionsBody({
  state,
  rejections,
  formatters
}: Readonly<{
  state: RejectionsState;
  rejections: DiagnosticsRejections;
  formatters: DiagnosticsFormatters;
}>) {
  const { t } = useTranslation();

  if (state === "unavailable") {
    return (
      <p data-diagnostics-rollback-note>{t("diagnostics.rollback.rejections.unavailable")}</p>
    );
  }
  if (state === "none") {
    return <p data-diagnostics-rollback-note>{t("diagnostics.rollback.rejections.none")}</p>;
  }
  return (
    <ul data-diagnostics-rejection-list>
      {rejections.byReason.map((entry) => (
        <li key={entry.reason ?? ""} data-diagnostics-rejection-reason={entry.reason ?? ""}>
          <span data-diagnostics-rejection-scroll>
            {entry.reason === null ? (
              <span>{t("diagnostics.values.reasonUnrecorded")}</span>
            ) : (
              <code>{entry.reason}</code>
            )}
          </span>
          <span data-diagnostics-metric="rejection-count">
            {formatters.number.format(entry.count)}
          </span>
        </li>
      ))}
    </ul>
  );
}

function RejectionsReading({
  rejections,
  formatters
}: Readonly<{ rejections: DiagnosticsRejections; formatters: DiagnosticsFormatters }>) {
  const { t } = useTranslation();
  const duration = useDurationText();
  const state = rejectionsState(rejections);
  return (
    <ReadingItem
      id="rejections"
      titleKey="diagnostics.labels.rejections"
      state={state}
      badge={
        rejections.humanReviewRequired
          ? { tone: "neutral", labelKey: "diagnostics.values.humanReview" }
          : null
      }
    >
      <p data-diagnostics-rollback-reading>
        {t("diagnostics.rollback.rejections.reading", {
          window: duration(rejections.windowSeconds)
        })}
      </p>
      <RejectionsBody state={state} rejections={rejections} formatters={formatters} />
    </ReadingItem>
  );
}

export function RollbackSection({
  rollback,
  formatters
}: Readonly<{ rollback: DiagnosticsRollback | null; formatters: DiagnosticsFormatters }>) {
  const { t } = useTranslation();
  return (
    <DiagnosticsCard
      id="rollback"
      titleKey="diagnostics.sections.rollback.title"
      descriptionKey="diagnostics.sections.rollback.description"
    >
      {rollback === null ? (
        <p data-diagnostics-rollback-unavailable>{t("diagnostics.rollback.unavailable")}</p>
      ) : (
        <ul data-diagnostics-rollback-list>
          <StreakReading
            id="challenge-delivery"
            titleKey="diagnostics.labels.challengeDelivery"
            streak={rollback.challengeDelivery.streak}
            formatters={formatters}
          >
            <CountRow
              name="challenge-delivery-failed"
              labelKey="diagnostics.labels.failedDeliveries"
              value={rollback.challengeDelivery.failedDeliveries}
              formatters={formatters}
            />
            <CountRow
              name="challenge-delivery-duplicate"
              labelKey="diagnostics.labels.duplicateDeliveries"
              value={rollback.challengeDelivery.duplicateDeliveries}
              formatters={formatters}
            />
          </StreakReading>
          <StreakReading
            id="console-access"
            titleKey="diagnostics.labels.consoleAccess"
            streak={rollback.consoleAccess.streak}
            formatters={formatters}
          >
            <CountRow
              name="console-access-attempts"
              labelKey="diagnostics.labels.unavailableAttempts"
              value={rollback.consoleAccess.unavailableAttempts}
              formatters={formatters}
            />
          </StreakReading>
          <DatabaseWritesReading writes={rollback.databaseWrites} formatters={formatters} />
          <RejectionsReading rejections={rollback.rejections} formatters={formatters} />
        </ul>
      )}
    </DiagnosticsCard>
  );
}
