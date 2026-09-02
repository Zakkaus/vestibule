import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import { Icon } from "../../icons";
import type { SettingSource } from "./api";

const sourceMessageKeys: Readonly<Record<SettingSource, string>> = {
  "factory default": "capabilities.source.factoryDefault",
  "user file": "capabilities.source.userFile",
  "chat override": "capabilities.source.chatOverride"
};

export function SourceMeta({
  source,
  pending,
  restoring,
  saving,
  allowRestore,
  onToggleRestore
}: Readonly<{
  source: SettingSource;
  pending?: boolean;
  restoring?: boolean;
  saving?: boolean;
  allowRestore?: boolean;
  onToggleRestore?: () => void;
}>) {
  const { t } = useTranslation();
  const sourceText = t(sourceMessageKeys[source]);

  return (
    <span data-capability-meta>
      <span
        data-slot={source === "factory default" ? undefined : "badge"}
        data-status={source === "chat override" ? "info" : "neutral"}
        data-capability-source={source}
      >
        <Icon name={source === "chat override" ? "info" : "circleMinus"} />
        {t("capabilities.source.value", { source: sourceText })}
      </span>
      {pending ? (
        <span data-slot="badge" data-status="pending" data-capability-pending>
          <Icon name="loaderCircle" />
          {t(restoring ? "capabilities.source.restoring" : "capabilities.source.pending")}
        </span>
      ) : null}
      {allowRestore && source === "chat override" && onToggleRestore ? (
        <button
          type="button"
          data-slot="button"
          data-variant="link"
          data-size="sm"
          aria-disabled={saving ? "true" : undefined}
          onClick={() => {
            if (!saving) {
              onToggleRestore();
            }
          }}
        >
          <Icon name={restoring ? "x" : "rotateCcw"} />
          {t(restoring ? "capabilities.actions.cancelRestore" : "capabilities.actions.restore")}
        </button>
      ) : null}
    </span>
  );
}

export function CapabilityCard({
  id,
  titleKey,
  summaryKey,
  enabled,
  source,
  onKey,
  offKey,
  detailsPath,
  detailsKey,
  groupSearch,
  sourceMeta,
  control
}: Readonly<{
  id: string;
  titleKey: string;
  summaryKey: string;
  enabled: boolean;
  source: SettingSource;
  onKey: string;
  offKey: string;
  detailsPath: string;
  detailsKey: string;
  groupSearch: string;
  sourceMeta?: ReactNode;
  control?: ReactNode;
}>) {
  const { t } = useTranslation();
  const titleID = `capabilities-${id}-title`;

  return (
    <section data-slot="card" data-capability-card={id} aria-labelledby={titleID}>
      <header data-capability-header>
        <div data-capability-copy>
          <div data-capability-title-row>
            <h2 id={titleID}>{t(titleKey)}</h2>
            <span data-slot="badge" data-status={enabled ? "ok" : "neutral"}>
              <Icon name={enabled ? "circleCheck" : "circleMinus"} />
              {t(enabled ? "capabilities.status.enabled" : "capabilities.status.disabled")}
            </span>
          </div>
          <p id={`capabilities-${id}-summary`}>{t(summaryKey)}</p>
          {sourceMeta ?? <SourceMeta source={source} />}
        </div>
        {control ? <div data-capability-control>{control}</div> : null}
      </header>
      <div data-capability-outcomes aria-label={t("capabilities.outcomes.label")}>
        <div data-capability-outcome="enabled">
          <span data-slot="badge" data-status="ok">
            <Icon name="circleCheck" />
            {t("capabilities.outcomes.enabled")}
          </span>
          <p>{t(onKey)}</p>
        </div>
        <div data-capability-outcome="disabled">
          <span data-slot="badge" data-status="neutral">
            <Icon name="circleMinus" />
            {t("capabilities.outcomes.disabled")}
          </span>
          <p>{t(offKey)}</p>
        </div>
      </div>
      <footer data-capability-footer>
        <Link
          to={{ pathname: detailsPath, search: groupSearch }}
          data-slot="button"
          data-variant="outline"
          data-size="sm"
        >
          <Icon name="arrowRight" />
          {t(detailsKey)}
        </Link>
      </footer>
    </section>
  );
}
