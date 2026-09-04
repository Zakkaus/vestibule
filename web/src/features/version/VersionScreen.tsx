import { useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Navigate } from "react-router-dom";

import {
  canViewInstanceStatus,
  retryConsoleSession,
  useConsoleSession
} from "../../app/session";
import { StatusBadge, type StatusTone } from "../../components/StatusBadge";
import { Icon, type IconName } from "../../icons";
import type { ApiRequestError } from "../../lib/api";
import type { LatestRelease, ReplacementResult, VersionStatus } from "./api";
import {
  useVersionController,
  type VersionController
} from "./useVersionController";

const replacementStatusKeys: Readonly<Record<string, string>> = {
  applied: "version.replacement.status.applied",
  failed: "version.replacement.status.failed",
  rejected: "version.replacement.status.rejected",
  rolled_back: "version.replacement.status.rolledBack",
  rollback_failed: "version.replacement.status.rollbackFailed"
};

const replacementReasonKeys: Readonly<Record<string, string>> = {
  complete: "version.replacement.reason.complete",
  request_claim_failed: "version.replacement.reason.requestClaimFailed",
  invalid_deployment: "version.replacement.reason.invalidDeployment",
  invalid_request: "version.replacement.reason.invalidRequest",
  apply_failed: "version.replacement.reason.applyFailed",
  healthcheck_failed: "version.replacement.reason.healthcheckFailed"
};

function VersionStateCard({
  id,
  titleKey,
  descriptionKey,
  icon,
  role,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  icon: IconName;
  role?: "alert" | "status";
  children?: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <section
      data-slot="card"
      data-version-state-card={id}
      role={role}
      aria-labelledby={`version-${id}-title`}
    >
      <h2 id={`version-${id}-title`} data-state-heading>
        <Icon name={icon} />
        {t(titleKey)}
      </h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

function VersionSection({
  id,
  titleKey,
  descriptionKey,
  children
}: Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  children: ReactNode;
}>) {
  const { t } = useTranslation();
  return (
    <section data-slot="card" data-version-section={id} aria-labelledby={`version-${id}-title`}>
      <header data-version-section-heading>
        <div>
          <h2 id={`version-${id}-title`}>{t(titleKey)}</h2>
          <p>{t(descriptionKey)}</p>
        </div>
      </header>
      {children}
    </section>
  );
}

function errorMessageKey(error: ApiRequestError, scope: "status" | "release" | "upgrade"): string {
  if (error.kind === "network") {
    return `version.errors.${scope}Network`;
  }
  if (error.kind === "api") {
    const keyByCode: Readonly<Record<string, string>> = {
      authentication_expired: "version.errors.authenticationExpired",
      authentication_invalid: "version.errors.authenticationInvalid",
      csrf_invalid: "version.errors.csrfInvalid",
      release_lookup_access_denied: "version.errors.accessDenied",
      upgrade_access_denied: "version.errors.accessDenied",
      diagnostics_unavailable: "version.errors.statusUnavailable",
      release_lookup_unavailable: "version.errors.releaseUnavailable",
      upgrade_unavailable: "version.errors.upgradeUnavailable"
    };
    return keyByCode[error.code] ?? `version.errors.${scope}Unavailable`;
  }
  return `version.errors.${scope}InvalidResponse`;
}

function replacementTone(status: string): StatusTone {
  if (status === "applied") {
    return "ok";
  }
  if (status === "rolled_back") {
    return "pending";
  }
  return "error";
}

function ReplacementResultNotice({ result }: Readonly<{ result: ReplacementResult }>) {
  const { t } = useTranslation();
  const statusKey = replacementStatusKeys[result.status] ?? "version.replacement.status.unknown";
  const reasonKey = replacementReasonKeys[result.reason] ?? "version.replacement.reason.unknown";
  return (
    <aside data-version-replacement-result={result.status} aria-labelledby="version-last-result-title">
      <header>
        <h3 id="version-last-result-title">{t("version.replacement.title")}</h3>
        <StatusBadge tone={replacementTone(result.status)}>{t(statusKey)}</StatusBadge>
      </header>
      <p>{t(reasonKey, { reason: result.reason })}</p>
      <dl>
        <div>
          <dt>{t("version.replacement.requestedVersion")}</dt>
          <dd><code>{result.requestedVersion}</code></dd>
        </div>
        <div>
          <dt>{t("version.replacement.reasonCode")}</dt>
          <dd><code>{result.reason || t("version.values.none")}</code></dd>
        </div>
      </dl>
    </aside>
  );
}

function CurrentVersionSection({ status }: Readonly<{ status: VersionStatus }>) {
  const { t } = useTranslation();
  return (
    <VersionSection
      id="current"
      titleKey="version.current.title"
      descriptionKey="version.current.description"
    >
      <dl data-version-current-values>
        <div>
          <dt>{t("version.current.version")}</dt>
          <dd><code data-version-current>{status.version}</code></dd>
        </div>
        <div>
          <dt>{t("version.current.hostUnit")}</dt>
          <dd>
            <StatusBadge tone={status.replacement.unitAvailable ? "ok" : "neutral"}>
              {t(status.replacement.unitAvailable
                ? "version.current.hostUnitAvailable"
                : "version.current.hostUnitUnavailable")}
            </StatusBadge>
          </dd>
        </div>
      </dl>
      {status.replacement.lastResult ? (
        <ReplacementResultNotice result={status.replacement.lastResult} />
      ) : (
        <p data-version-no-result>{t("version.replacement.none")}</p>
      )}
    </VersionSection>
  );
}

function RollbackNotice({ release }: Readonly<{ release: LatestRelease }>) {
  const { t } = useTranslation();
  const rollback = release.rollback;
  if (!rollback) {
    return null;
  }
  const variables = {
    target: rollback.targetSchemaVersion,
    retained: rollback.retainedSchemaVersion,
    minimum: rollback.minimumRollbackSchemaVersion
  };
  let descriptionKey = "version.rollback.unknown";
  if (rollback.available) {
    descriptionKey = "version.rollback.available";
  } else if (rollback.reason === "schema_incompatible") {
    descriptionKey = "version.rollback.schemaIncompatible";
  } else if (rollback.reason === "unknown_target_schema") {
    descriptionKey = "version.rollback.unknownTarget";
  } else if (rollback.reason === "not_an_earlier_schema") {
    descriptionKey = "version.rollback.notEarlier";
  }
  return (
    <aside data-version-rollback={rollback.available ? "available" : "blocked"}>
      <StatusBadge tone={rollback.available ? "ok" : "error"}>
        {t(rollback.available ? "version.rollback.safe" : "version.rollback.blocked")}
      </StatusBadge>
      <p>{t(descriptionKey, variables)}</p>
    </aside>
  );
}

function ManualUpgradeNotice({ version }: Readonly<{ version: string }>) {
  const { t } = useTranslation();
  return (
    <aside data-version-manual-upgrade aria-labelledby="version-manual-title">
      <h3 id="version-manual-title">
        <Icon name="settings" />
        {t("version.manual.title")}
      </h3>
      <p>{t("version.manual.description")}</p>
      <p>{t("version.manual.imageLabel")}</p>
      <code data-version-manual-image>{`VESTIBULE_APP_IMAGE=ghcr.io/zakkaus/vestibule:${version}`}</code>
      <p>{t("version.manual.commandsLabel")}</p>
      <pre data-version-manual-command><code>{"docker compose --env-file container.env pull app\ndocker compose --env-file container.env up -d --no-deps app"}</code></pre>
    </aside>
  );
}

function UpgradeProgress({ controller }: Readonly<{ controller: VersionController }>) {
  const { t } = useTranslation();
  const { upgradeState } = controller;
  if (upgradeState.kind !== "requesting" && upgradeState.kind !== "monitoring") {
    return null;
  }
  return (
    <div data-version-upgrade-progress={upgradeState.kind} role="status">
      <button
        type="button"
        data-slot="button"
        data-version-action="upgrade"
        aria-disabled="true"
        onClick={controller.confirmUpgrade}
      >
        <Icon name="loaderCircle" />
        {t(upgradeState.kind === "requesting"
          ? "version.upgrade.requesting"
          : "version.upgrade.monitoring")}
      </button>
      <p>{t("version.upgrade.restartNotice")}</p>
    </div>
  );
}

function UpgradeFeedback({ controller }: Readonly<{ controller: VersionController }>) {
  const { t } = useTranslation();
  const { upgradeState } = controller;
  if (upgradeState.kind === "applied" || upgradeState.kind === "failed") {
    return (
      <div data-version-upgrade-outcome={upgradeState.kind} role={upgradeState.kind === "failed" ? "alert" : "status"}>
        <ReplacementResultNotice result={upgradeState.result} />
        {upgradeState.kind === "failed" ? (
          <button
            type="button"
            data-slot="button"
            data-variant="outline"
            onClick={() => controller.beginUpgrade(upgradeState.result.requestedVersion)}
          >
            <Icon name="rotateCcw" />
            {t("version.upgrade.tryAgain")}
          </button>
        ) : null}
      </div>
    );
  }
  if (upgradeState.kind === "request-unavailable" || upgradeState.kind === "monitor-unavailable") {
    const descriptionKey = upgradeState.kind === "request-unavailable"
      ? errorMessageKey(upgradeState.error, "upgrade")
      : "version.errors.monitorUnavailable";
    return (
      <div data-version-upgrade-outcome="unavailable" role="alert">
        <p>{t(descriptionKey)}</p>
        <div data-button-row>
          <button type="button" data-slot="button" data-variant="outline" onClick={controller.retryUpgrade}>
            <Icon name="refreshCw" />
            {t("version.actions.retry")}
          </button>
          <button type="button" data-slot="button" data-variant="ghost" onClick={controller.cancelUpgrade}>
            <Icon name="x" />
            {t("version.actions.cancel")}
          </button>
        </div>
      </div>
    );
  }
  return <UpgradeProgress controller={controller} />;
}

function UpgradeControls({ version, controller }: Readonly<{ version: string; controller: VersionController }>) {
  const { t } = useTranslation();
  const { upgradeState } = controller;
  if (upgradeState.kind === "confirming") {
    return (
      <div data-version-upgrade-confirmation role="alert">
        <p>{t("version.upgrade.confirm", { version })}</p>
        <div data-button-row>
          <button type="button" data-slot="button" data-version-action="upgrade-confirm" onClick={controller.confirmUpgrade}>
            <Icon name="refreshCw" />
            {t("version.upgrade.confirmAction")}
          </button>
          <button type="button" data-slot="button" data-variant="ghost" onClick={controller.cancelUpgrade}>
            <Icon name="x" />
            {t("version.actions.cancel")}
          </button>
        </div>
      </div>
    );
  }
  if (upgradeState.kind !== "idle") {
    return <UpgradeFeedback controller={controller} />;
  }
  return (
    <button
      type="button"
      data-slot="button"
      data-version-action="upgrade"
      onClick={() => controller.beginUpgrade(version)}
    >
      <Icon name="refreshCw" />
      {t("version.upgrade.action")}
    </button>
  );
}

function UpgradePath({
  status,
  release,
  controller
}: Readonly<{ status: VersionStatus; release: LatestRelease; controller: VersionController }>) {
  const { t } = useTranslation();
  if (!release.updateAvailable || !release.rollback) {
    return null;
  }
  if (!status.replacement.unitAvailable) {
    return <ManualUpgradeNotice version={release.version} />;
  }
  if (!release.rollback.available) {
    return <p data-version-upgrade-blocked>{t("version.upgrade.rollbackBlocked")}</p>;
  }
  return <UpgradeControls version={release.version} controller={controller} />;
}

function ReleaseDetails({
  status,
  release,
  controller,
  dateFormatter
}: Readonly<{
  status: VersionStatus;
  release: LatestRelease;
  controller: VersionController;
  dateFormatter: Intl.DateTimeFormat;
}>) {
  const { t } = useTranslation();
  return (
    <div data-version-release-details>
      <div data-version-release-summary>
        <div>
          <strong><code data-version-latest>{release.version}</code></strong>
          <time dateTime={release.publishedAt}>{dateFormatter.format(new Date(release.publishedAt))}</time>
        </div>
        <StatusBadge tone={release.updateAvailable ? "info" : "ok"}>
          {t(release.updateAvailable ? "version.release.available" : "version.release.current")}
        </StatusBadge>
      </div>
      <div data-version-release-notes>
        <h3>{t("version.release.notes")}</h3>
        <p>{release.notes || t("version.release.noNotes")}</p>
        <a href={release.url} target="_blank" rel="noreferrer">
          <Icon name="bookOpen" />
          {t("version.release.open")}
        </a>
      </div>
      <RollbackNotice release={release} />
      <UpgradePath status={status} release={release} controller={controller} />
    </div>
  );
}

function ReleaseSection({
  status,
  controller,
  dateFormatter
}: Readonly<{
  status: VersionStatus;
  controller: VersionController;
  dateFormatter: Intl.DateTimeFormat;
}>) {
  const { t } = useTranslation();
  const { releaseState } = controller;
  const checking = releaseState.kind === "checking";
  return (
    <VersionSection
      id="release"
      titleKey="version.release.title"
      descriptionKey="version.release.description"
    >
      <div data-version-release-actions>
        <button
          type="button"
          data-slot="button"
          data-variant="outline"
          data-version-action="check"
          aria-disabled={checking || undefined}
          onClick={controller.checkLatest}
        >
          <Icon name={checking ? "loaderCircle" : "refreshCw"} />
          {t(checking ? "version.release.checking" : "version.release.check")}
        </button>
        {releaseState.kind === "idle" ? <p>{t("version.release.notChecked")}</p> : null}
      </div>
      {releaseState.kind === "unavailable" ? (
        <div data-version-release-error role="alert">
          <p>{t(errorMessageKey(releaseState.error, "release"))}</p>
          <button type="button" data-slot="button" data-variant="outline" onClick={controller.checkLatest}>
            <Icon name="refreshCw" />
            {t("version.actions.retry")}
          </button>
        </div>
      ) : null}
      {releaseState.kind === "loaded" ? (
        <ReleaseDetails
          status={status}
          release={releaseState.release}
          controller={controller}
          dateFormatter={dateFormatter}
        />
      ) : null}
    </VersionSection>
  );
}

function OperatorVersionScreen() {
  const { i18n, t } = useTranslation();
  const controller = useVersionController();
  const locale = i18n.resolvedLanguage ?? i18n.language;
  const dateFormatter = useMemo(() => new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit"
  }), [locale]);
  const { baseState } = controller;
  return (
    <section
      data-version-page
      data-version-state={baseState.kind}
      aria-busy={baseState.kind === "loading" || undefined}
      aria-labelledby="version-title"
    >
      <header data-page-heading>
        <h1 id="version-title">{t("version.title")}</h1>
        <p>{t("version.description")}</p>
      </header>
      {baseState.kind === "loading" ? (
        <VersionStateCard id="loading" titleKey="version.loading.title" descriptionKey="version.loading.description" icon="loaderCircle" role="status" />
      ) : null}
      {baseState.kind === "access-denied" ? (
        <VersionStateCard id="access-denied" titleKey="version.accessDenied.title" descriptionKey="version.accessDenied.description" icon="circleAlert" role="alert" />
      ) : null}
      {baseState.kind === "unavailable" ? (
        <VersionStateCard id="unavailable" titleKey="version.unavailable.title" descriptionKey={errorMessageKey(baseState.error, "status")} icon="circleAlert" role="alert">
          <button type="button" data-slot="button" data-variant="outline" onClick={controller.reloadStatus}>
            <Icon name="refreshCw" />
            {t("version.actions.retry")}
          </button>
        </VersionStateCard>
      ) : null}
      {baseState.kind === "loaded" ? (
        <div data-version-content>
          <CurrentVersionSection status={baseState.status} />
          <ReleaseSection status={baseState.status} controller={controller} dateFormatter={dateFormatter} />
        </div>
      ) : null}
    </section>
  );
}

function PermissionPendingScreen({ unavailable }: Readonly<{ unavailable: boolean }>) {
  const { t } = useTranslation();
  const stateID = unavailable ? "session-unavailable" : "permission-loading";
  return (
    <section
      data-permission-pending={stateID}
      aria-busy={!unavailable || undefined}
      aria-labelledby={`version-${stateID}-title`}
    >
      <VersionStateCard
        id={stateID}
        titleKey={unavailable
          ? "version.loading.permissionUnavailableTitle"
          : "version.loading.permissionTitle"}
        descriptionKey={unavailable
          ? "version.errors.authenticationUnavailable"
          : "version.loading.permissionDescription"}
        icon={unavailable ? "circleAlert" : "loaderCircle"}
        role={unavailable ? "alert" : "status"}
      >
        {unavailable ? (
          <button
            type="button"
            data-slot="button"
            data-variant="outline"
            onClick={() => void retryConsoleSession()}
          >
            <Icon name="refreshCw" />
            {t("version.actions.retry")}
          </button>
        ) : null}
      </VersionStateCard>
    </section>
  );
}

export function VersionScreen() {
  const session = useConsoleSession();
  if ("session" in session && !canViewInstanceStatus(session)) {
    return <Navigate to="/groups" replace />;
  }
  if (!canViewInstanceStatus(session)) {
    return <PermissionPendingScreen unavailable={session.state === "blocked"} />;
  }
  return <OperatorVersionScreen />;
}
