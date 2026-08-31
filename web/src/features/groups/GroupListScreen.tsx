import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import { StatusBadge, type StatusTone } from "../../components/StatusBadge";
import {
  allGroupsSelection,
  groupFixtures,
  modeDefinitions,
  resolveGroupSelection,
  verificationPrerequisites
} from "./fixtures";

export function GroupListScreen() {
  const { t, i18n } = useTranslation();
  const [searchParams] = useSearchParams();
  const selectedGroupId = resolveGroupSelection(searchParams.get("group"));
  const numberFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language),
    [i18n.language, i18n.resolvedLanguage]
  );

  return (
    <section data-groups-page aria-labelledby="groups-title">
      <header data-page-heading>
        <div data-group-heading>
          <h1 id="groups-title">{t("groups.title")}</h1>
          <StatusBadge tone="neutral">
            {t("groups.managedCount", { count: groupFixtures.length })}
          </StatusBadge>
        </div>
        <p>{t("groups.description")}</p>
      </header>


      <div data-group-list>
        {groupFixtures.map((group) => {
          const mode = modeDefinitions[group.mode];
          const missingPrerequisites = verificationPrerequisites.filter(
            (prerequisite) => !group.prerequisites[prerequisite.id]
          );
          const verificationTone: StatusTone = missingPrerequisites.length === 0 ? "ok" : "error";
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
            <article
              key={group.id}
              data-slot="card"
              data-group-row
              data-selected={selectedGroupId === group.id ? "" : undefined}
              data-verifiable={missingPrerequisites.length === 0 ? "true" : "false"}
            >
              <div data-group-primary>
                <div data-group-heading>
                  <h2>{t(group.nameKey)}</h2>
                  <StatusBadge tone="neutral">{t(mode.labelKey)}</StatusBadge>
                  <StatusBadge tone={verificationTone}>
                    {t(
                      missingPrerequisites.length === 0
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
                    <span data-group-metric-value>
                      {numberFormatter.format(expiredCount)}
                    </span>
                  </div>
                </div>

                <ul data-settlement-list>
                  {group.settlements.map((settlement) => (
                    <li key={settlement.result.id} data-settlement-item>
                      <span>{t(settlement.result.labelKey)}</span>
                      <span data-settlement-value>
                        {numberFormatter.format(settlement.count)}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>

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
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}
