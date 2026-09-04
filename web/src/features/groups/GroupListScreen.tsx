import { type ReactNode, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import {
  retryConsoleGroups,
  type ConsoleChat,
  useConsoleSession
} from "../../app/session";
import { StatusBadge, type StatusTone } from "../../components/StatusBadge";
import { Icon } from "../../icons";
import type { ApiRequestError } from "../../lib/api";
import {
  groupFixtures,
  type GroupFixture,
  isGroupFixtureFallback,
  modeDefinitions,
  resolveGroupSelection,
  verificationPrerequisites
} from "./fixtures";

type GroupsPageProps = Readonly<{
  state: "loading" | "populated" | "empty" | "error";
  source?: "api" | "fixtures";
  count?: number;
  children: ReactNode;
}>;

function GroupsPage({ state, source, count, children }: GroupsPageProps) {
  const { t } = useTranslation();

  return (
    <section
      data-groups-page
      data-groups-state={state}
      data-groups-source={source}
      aria-busy={state === "loading" ? "true" : undefined}
      aria-labelledby="groups-title"
    >
      <header data-page-heading>
        <div data-group-heading>
          <h1 id="groups-title">{t("groups.title")}</h1>
          {count === undefined ? null : (
            <StatusBadge tone="neutral">
              {t("groups.managedCount", { count })}
            </StatusBadge>
          )}
        </div>
        <p>{t("groups.description")}</p>
      </header>
      <div data-group-list>{children}</div>
    </section>
  );
}

type GroupStateCardProps = Readonly<{
  state: "loading" | "empty" | "error";
  title: string;
  description: string;
  action?: ReactNode;
  errorCode?: string;
}>;

const groupStateIcon: Readonly<Record<GroupStateCardProps["state"], "loaderCircle" | "usersRound" | "circleAlert">> = {
  loading: "loaderCircle",
  empty: "usersRound",
  error: "circleAlert"
};

function GroupStateCard({
  state,
  title,
  description,
  action,
  errorCode
}: GroupStateCardProps) {
  return (
    <section
      data-slot="card"
      data-group-state={state}
      data-group-error-code={errorCode}
      role={state === "error" ? "alert" : "status"}
    >
      <h2>
        <span data-state-heading>
          <Icon name={groupStateIcon[state]} />
          {title}
        </span>
      </h2>
      <p>{description}</p>
      {action ? <div data-group-state-actions>{action}</div> : null}
    </section>
  );
}

function OpenQueueLink({ groupId, groupName }: Readonly<{ groupId: string; groupName: string }>) {
  const { t } = useTranslation();
  const query = new URLSearchParams({ group: groupId });

  return (
    <Link
      data-slot="button"
      data-size="sm"
      data-variant="primary"
      data-select-group={groupId}
      to={`/queue?${query.toString()}`}
      aria-label={t("groups.actions.openQueueFor", { group: groupName })}
    >
      <Icon name="arrowRight" />
      {t("groups.actions.openQueue")}
    </Link>
  );
}

function LiveGroupList({
  chats,
  selectedGroupId
}: Readonly<{ chats: readonly ConsoleChat[]; selectedGroupId: string }>) {
  const { t } = useTranslation();

  return chats.map((chat) => {
    const groupName = chat.title ?? t("groups.groupOption", { id: chat.id });

    return (
      <article
        key={chat.id}
        data-slot="card"
        data-group-row
        data-selected={selectedGroupId === chat.id ? "" : undefined}
      >
        <div data-group-primary>
          <div data-group-heading>
            <h2>{groupName}</h2>
            <StatusBadge tone="neutral">{t("groups.authorized")}</StatusBadge>
          </div>
          <p data-live-group-note>{t("groups.liveGroupNote")}</p>
        </div>
        <div data-group-actions>
          <OpenQueueLink groupId={chat.id} groupName={groupName} />
        </div>
      </article>
    );
  });
}

type FixtureGroupPrimaryProps = Readonly<{
  group: GroupFixture;
  missingPrerequisiteCount: number;
  numberFormatter: Intl.NumberFormat;
}>;

function FixtureGroupPrimary({
  group,
  missingPrerequisiteCount,
  numberFormatter
}: FixtureGroupPrimaryProps) {
  const { t } = useTranslation();
  const mode = modeDefinitions[group.mode];
  const verificationTone: StatusTone = missingPrerequisiteCount === 0 ? "ok" : "error";
  const settledCount = group.settlements.reduce(
    (count, settlement) => count + settlement.count,
    0
  );
  const expiredCount = group.settlements.reduce(
    (count, settlement) =>
      settlement.result.state === "expired" ? count + settlement.count : count,
    0
  );

  return (
    <div data-group-primary>
      <div data-group-heading>
        <h2>{t(group.nameKey)}</h2>
        <StatusBadge tone="neutral">{t(mode.labelKey)}</StatusBadge>
        <StatusBadge tone={verificationTone}>
          {t(
            missingPrerequisiteCount === 0
              ? "groups.verification.available"
              : "groups.verification.unavailable"
          )}
        </StatusBadge>
      </div>
      <span data-group-id>{group.id}</span>
      {mode.noteKey ? <p data-mode-note>{t(mode.noteKey)}</p> : null}
      <div data-group-metrics>
        <div data-group-metric>
          <span data-group-metric-label>{t("groups.metrics.applications48h")}</span>
          <span data-group-metric-value>
            {numberFormatter.format(group.applicationsLast48Hours)}
          </span>
        </div>
        <div data-group-metric>
          <span data-group-metric-label>{t("groups.metrics.settled")}</span>
          <span data-group-metric-value>{numberFormatter.format(settledCount)}</span>
        </div>
        <div data-group-metric>
          <span data-group-metric-label>{t("groups.metrics.expired")}</span>
          <span data-group-metric-value>{numberFormatter.format(expiredCount)}</span>
        </div>
      </div>
      <ul data-settlement-list>
        {group.settlements.map((settlement) => (
          <li key={settlement.result.id} data-settlement-item>
            <span>{t(settlement.result.labelKey)}</span>
            <span data-settlement-value>{numberFormatter.format(settlement.count)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function FixtureGroupDetails({ group }: Readonly<{ group: GroupFixture }>) {
  const { t } = useTranslation();

  return (
    <div data-group-side>
      <section aria-labelledby={`prerequisites-${group.id}`}>
        <h3 id={`prerequisites-${group.id}`} data-prerequisite-heading>
          {t("groups.prerequisites.heading")}
        </h3>
        <ul data-prerequisite-list>
          {verificationPrerequisites.map((prerequisite) => {
            const isPresent = group.prerequisites[prerequisite.id];

            return (
              <li
                key={prerequisite.id}
                data-prerequisite
                data-prerequisite-state={isPresent ? "present" : "missing"}
              >
                <span>{t(prerequisite.labelKey)}</span>
                <StatusBadge tone={isPresent ? "ok" : "error"}>
                  {t(isPresent ? "groups.prerequisites.present" : "groups.prerequisites.missing")}
                </StatusBadge>
              </li>
            );
          })}
        </ul>
      </section>
      <section aria-labelledby={`applicants-${group.id}`}>
        <h3 id={`applicants-${group.id}`} data-applicant-heading>
          {t("groups.applicants.heading")}
        </h3>
        <ul data-applicant-list>
          {group.recentApplicants.map((applicant) => (
            <li key={applicant.userId} data-applicant>
              <span data-applicant-identity>
                {applicant.username
                  ? t("groups.applicants.withUsername", {
                      username: applicant.username,
                      id: applicant.userId
                    })
                  : t("groups.applicants.idOnly", { id: applicant.userId })}
              </span>
              <StatusBadge tone={applicant.result.tone}>
                {t(applicant.result.labelKey)}
              </StatusBadge>
            </li>
          ))}
        </ul>
      </section>
      <div data-group-actions>
        <OpenQueueLink groupId={group.id} groupName={t(group.nameKey)} />
      </div>
    </div>
  );
}

function FixtureGroupList({ selectedGroupId }: Readonly<{ selectedGroupId: string }>) {
  const { i18n } = useTranslation();
  const numberFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language),
    [i18n.language, i18n.resolvedLanguage]
  );

  return groupFixtures.map((group) => {
    const missingPrerequisiteCount = verificationPrerequisites.filter(
      (prerequisite) => !group.prerequisites[prerequisite.id]
    ).length;

    return (
      <article
        key={group.id}
        data-slot="card"
        data-group-row
        data-selected={selectedGroupId === group.id ? "" : undefined}
        data-verifiable={missingPrerequisiteCount === 0 ? "true" : "false"}
      >
        <FixtureGroupPrimary
          group={group}
          missingPrerequisiteCount={missingPrerequisiteCount}
          numberFormatter={numberFormatter}
        />
        <FixtureGroupDetails group={group} />
      </article>
    );
  });
}

type GroupErrorPresentation = Readonly<{
  code: string;
  titleKey: string;
  descriptionKey: string;
  retryable: boolean;
}>;

function groupErrorPresentation(error: ApiRequestError): GroupErrorPresentation {
  if (error.kind === "network") {
    return {
      code: error.kind,
      titleKey: "groups.error.network.title",
      descriptionKey: "groups.error.network.description",
      retryable: true
    };
  }
  if (error.kind !== "api") {
    return {
      code: error.kind,
      titleKey: "groups.error.response.title",
      descriptionKey: "groups.error.response.description",
      retryable: true
    };
  }

  switch (error.code) {
    case "authentication_expired":
      return {
        code: error.code,
        titleKey: "groups.error.authenticationExpired.title",
        descriptionKey: "groups.error.authenticationExpired.description",
        retryable: false
      };
    case "authentication_invalid":
      return {
        code: error.code,
        titleKey: "groups.error.authenticationInvalid.title",
        descriptionKey: "groups.error.authenticationInvalid.description",
        retryable: false
      };
    case "init_data_replayed":
      return {
        code: error.code,
        titleKey: "groups.error.initDataReplayed.title",
        descriptionKey: "groups.error.initDataReplayed.description",
        retryable: false
      };
    case "authentication_unavailable":
      return {
        code: error.code,
        titleKey: "groups.error.authenticationUnavailable.title",
        descriptionKey: "groups.error.authenticationUnavailable.description",
        retryable: true
      };
    case "verification_unavailable":
      return {
        code: error.code,
        titleKey: "groups.error.verificationUnavailable.title",
        descriptionKey: "groups.error.verificationUnavailable.description",
        retryable: true
      };
    default:
      return {
        code: error.code,
        titleKey: "groups.error.unknown.title",
        descriptionKey: "groups.error.unknown.description",
        retryable: true
      };
  }
}

function GroupsUnavailable({
  error,
  canRetry
}: Readonly<{ error: ApiRequestError; canRetry: boolean }>) {
  const { t } = useTranslation();
  const presentation = groupErrorPresentation(error);
  const action =
    canRetry && presentation.retryable ? (
      <button
        data-slot="button"
        data-size="sm"
        data-variant="primary"
        type="button"
        onClick={() => void retryConsoleGroups()}
      >
        <Icon name="refreshCw" />
        {t("groups.actions.retry")}
      </button>
    ) : undefined;

  return (
    <GroupsPage state="error">
      <GroupStateCard
        state="error"
        errorCode={presentation.code}
        title={t(presentation.titleKey)}
        description={t(presentation.descriptionKey)}
        action={action}
      />
    </GroupsPage>
  );
}

export function GroupListScreen() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const session = useConsoleSession();

  if (isGroupFixtureFallback(session)) {
    const selectedGroupId = resolveGroupSelection(searchParams.get("group"), groupFixtures);
    return (
      <GroupsPage state="populated" source="fixtures" count={groupFixtures.length}>
        <FixtureGroupList selectedGroupId={selectedGroupId} />
      </GroupsPage>
    );
  }

  if (session.state === "loading" || session.state === "checking-groups") {
    return (
      <GroupsPage state="loading">
        <GroupStateCard
          state="loading"
          title={t("groups.loading.title")}
          description={t("groups.loading.description")}
        />
      </GroupsPage>
    );
  }

  if (session.state === "ready") {
    const selectedGroupId = resolveGroupSelection(searchParams.get("group"), session.chats);
    return (
      <GroupsPage state="populated" source="api" count={session.chats.length}>
        <LiveGroupList chats={session.chats} selectedGroupId={selectedGroupId} />
      </GroupsPage>
    );
  }

  if (session.state === "no-groups") {
    return (
      <GroupsPage state="empty" source="api" count={0}>
        <GroupStateCard
          state="empty"
          title={t("groups.empty.title")}
          description={t("groups.empty.description", {
            accountId: session.session.subject.telegramId
          })}
        />
      </GroupsPage>
    );
  }

  if (session.state === "groups-unavailable") {
    return <GroupsUnavailable error={session.error} canRetry />;
  }

  return <GroupsUnavailable error={session.error} canRetry={false} />;
}
