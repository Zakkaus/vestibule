import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";

import { consoleApi, useConsoleSession } from "../../app/session";
import type { StatusTone } from "../../components/StatusBadge";
import type { ApiRequestError } from "../../lib/api";
import { loadAuditRecords, undoAuditRecord, type AuditRecord } from "./api";
import { auditFixtureFor, type AuditFixture } from "./fixtures";
import { AuditTable, type PendingAuditActions } from "./AuditTable";

const FIXTURE_ACTION_DELAY_MS = 700;
const FEEDBACK_DURATION_MS = 5_000;

const errorMessageKeys: Readonly<Record<string, string>> = {
  authentication_expired: "audit.errors.authenticationExpired",
  authentication_invalid: "audit.errors.authenticationInvalid",
  audit_conflict: "audit.errors.auditConflict",
  audit_not_found: "audit.errors.auditNotFound",
  audit_not_undoable: "audit.errors.auditNotUndoable",
  audit_unavailable: "audit.errors.auditUnavailable",
  chat_access_denied: "audit.errors.accessDenied",
  chat_access_unavailable: "audit.errors.accessUnavailable",
  chat_not_found: "audit.errors.chatNotFound",
  csrf_invalid: "audit.errors.csrfInvalid",
  invalid_audit: "audit.errors.invalidAudit"
};

const accessRevocationCodes: Readonly<Record<string, true>> = {
  authentication_expired: true,
  authentication_invalid: true,
  chat_access_denied: true,
  chat_not_found: true
};

type AuditScreenState =
  | Readonly<{ kind: "loading" }>
  | Readonly<{ kind: "fixture"; fixture: AuditFixture }>
  | Readonly<{ kind: "loaded" }>
  | Readonly<{ kind: "unavailable"; error: ApiRequestError }>
  | Readonly<{ kind: "group-required" }>
  | Readonly<{ kind: "no-groups" }>;

type AuditFeedback = Readonly<{
  id: number;
  messageKey: string;
  tone: StatusTone;
  record: AuditRecord;
}>;

function auditErrorMessageKey(error: ApiRequestError, fallback: string): string {
  if (error.kind === "network") {
    return "audit.errors.network";
  }
  if (error.kind === "api") {
    return errorMessageKeys[error.code] ?? fallback;
  }
  return fallback;
}

function AuditFeedbackNotice({ feedback }: Readonly<{ feedback: AuditFeedback }>) {
  const { t } = useTranslation();

  return (
    <div
      data-record-feedback
      data-audit-feedback
      data-tone={feedback.tone}
      role={feedback.tone === "error" ? "alert" : "status"}
      aria-atomic="true"
    >
      {t(feedback.messageKey, { user: feedback.record.user })}
    </div>
  );
}

type AuditStateCardProps = Readonly<{
  id: string;
  titleKey: string;
  descriptionKey: string;
  role?: "alert";
  live?: "polite";
  children?: ReactNode;
}>;

function AuditStateCard({
  id,
  titleKey,
  descriptionKey,
  role,
  live,
  children
}: AuditStateCardProps) {
  const { t } = useTranslation();

  return (
    <section
      data-slot="card"
      data-record-empty
      data-audit-state-card={id}
      role={role}
      aria-live={live}
      aria-labelledby={`audit-${id}-title`}
    >
      <h2 id={`audit-${id}-title`}>{t(titleKey)}</h2>
      <p>{t(descriptionKey)}</p>
      {children}
    </section>
  );
}

export function AuditScreen() {
  const { i18n, t } = useTranslation();
  const session = useConsoleSession();
  const [searchParams] = useSearchParams();
  const fixture = auditFixtureFor(searchParams.get("fixture"));
  const selectedGroupID = searchParams.get("group");
  const chatID =
    selectedGroupID !== null && /^-?\d+$/.test(selectedGroupID) ? selectedGroupID : undefined;
  const [auditState, setAuditState] = useState<AuditScreenState>({ kind: "loading" });
  const [records, setRecords] = useState<readonly AuditRecord[]>([]);
  const [pendingActions, setPendingActions] = useState<PendingAuditActions>({});
  const [feedback, setFeedback] = useState<AuditFeedback | null>(null);
  const [reloadVersion, setReloadVersion] = useState(0);
  const inFlightRecordIDsRef = useRef(new Set<string>());
  const activeScopeRef = useRef("");
  const feedbackSequenceRef = useRef(0);
  const feedbackTimerRef = useRef<number | undefined>(undefined);
  const fixtureTimerIDsRef = useRef(new Set<number>());
  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat(i18n.resolvedLanguage ?? i18n.language, {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hourCycle: "h23"
      }),
    [i18n.language, i18n.resolvedLanguage]
  );

  useEffect(() => {
    const scope = `${session.state}:${chatID ?? ""}`;
    activeScopeRef.current = scope;
    let active = true;
    setRecords([]);

    if (session.state === "loading" || session.state === "checking-groups") {
      setAuditState({ kind: "loading" });
      return () => {
        active = false;
      };
    }

    if (session.state === "blocked" && session.error.kind === "non-json") {
      setRecords([...fixture.records]);
      setAuditState({ kind: "fixture", fixture });
      return () => {
        active = false;
      };
    }

    if (session.state === "blocked" || session.state === "groups-unavailable") {
      setAuditState({ kind: "unavailable", error: session.error });
      return () => {
        active = false;
      };
    }

    if (session.state === "no-groups") {
      setAuditState({ kind: "no-groups" });
      return () => {
        active = false;
      };
    }

    if (!chatID) {
      setAuditState({ kind: "group-required" });
      return () => {
        active = false;
      };
    }

    setAuditState({ kind: "loading" });
    void loadAuditRecords(consoleApi, chatID).then((result) => {
      if (!active || activeScopeRef.current !== scope) {
        return;
      }
      if (result.ok) {
        setRecords(result.data);
        setAuditState({ kind: "loaded" });
        return;
      }
      setAuditState({ kind: "unavailable", error: result.error });
    });

    return () => {
      active = false;
    };
  }, [chatID, fixture, reloadVersion, session]);

  useEffect(() => {
    if (feedbackTimerRef.current !== undefined) {
      window.clearTimeout(feedbackTimerRef.current);
      feedbackTimerRef.current = undefined;
    }
    fixtureTimerIDsRef.current.forEach((timerID) => window.clearTimeout(timerID));
    fixtureTimerIDsRef.current.clear();
    inFlightRecordIDsRef.current.clear();
    setPendingActions({});
    setFeedback(null);

    return () => {
      if (feedbackTimerRef.current !== undefined) {
        window.clearTimeout(feedbackTimerRef.current);
        feedbackTimerRef.current = undefined;
      }
      fixtureTimerIDsRef.current.forEach((timerID) => window.clearTimeout(timerID));
      fixtureTimerIDsRef.current.clear();
      inFlightRecordIDsRef.current.clear();
    };
  }, [chatID, session.state]);

  function showFeedback(
    messageKey: string,
    tone: StatusTone,
    record: AuditRecord,
    dismissAfter: number | undefined
  ): void {
    if (feedbackTimerRef.current !== undefined) {
      window.clearTimeout(feedbackTimerRef.current);
      feedbackTimerRef.current = undefined;
    }
    const id = ++feedbackSequenceRef.current;
    setFeedback({ id, messageKey, tone, record });
    if (dismissAfter !== undefined) {
      const timerID = window.setTimeout(() => {
        if (feedbackSequenceRef.current === id) {
          setFeedback(null);
        }
        feedbackTimerRef.current = undefined;
      }, dismissAfter);
      feedbackTimerRef.current = timerID;
    }
  }

  function finishAction(recordID: string, scope: string): void {
    if (activeScopeRef.current !== scope) {
      return;
    }
    inFlightRecordIDsRef.current.delete(recordID);
    setPendingActions((currentActions) => {
      const nextActions = { ...currentActions };
      delete nextActions[recordID];
      return nextActions;
    });
  }

  function showUndoResult(record: AuditRecord): void {
    switch (record.undoState) {
      case "completed":
        showFeedback("audit.feedback.undoSuccess", "ok", record, FEEDBACK_DURATION_MS);
        break;
      case "pending":
        showFeedback("audit.feedback.undoPending", "info", record, FEEDBACK_DURATION_MS);
        break;
      case "failed":
        showFeedback("audit.feedback.undoFailure", "error", record, undefined);
        break;
      default:
        showFeedback("audit.errors.auditConflict", "error", record, undefined);
    }
  }

  function undoFixtureRecord(record: AuditRecord, scope: string, fixtureData: AuditFixture): void {
    const shouldFail =
      fixtureData.records.find((fixtureRecord) => fixtureRecord.id === record.id)
        ?.simulatedUndoOutcome === "failure";
    const timerID = window.setTimeout(() => {
      fixtureTimerIDsRef.current.delete(timerID);
      if (activeScopeRef.current !== scope) {
        return;
      }
      if (shouldFail) {
        showFeedback("audit.feedback.undoFailure", "error", record, undefined);
      } else {
        const completedRecord: AuditRecord = { ...record, undoState: "completed" };
        setRecords((currentRecords) =>
          currentRecords.map((currentRecord) =>
            currentRecord.id === record.id ? completedRecord : currentRecord
          )
        );
        showUndoResult(completedRecord);
      }
      finishAction(record.id, scope);
    }, FIXTURE_ACTION_DELAY_MS);
    fixtureTimerIDsRef.current.add(timerID);
  }

  function undoRecord(record: AuditRecord): void {
    if (record.undoState !== "available" || inFlightRecordIDsRef.current.has(record.id)) {
      return;
    }
    const scope = activeScopeRef.current;
    inFlightRecordIDsRef.current.add(record.id);
    setPendingActions((currentActions) => ({ ...currentActions, [record.id]: true }));
    setFeedback(null);

    if (auditState.kind === "fixture") {
      undoFixtureRecord(record, scope, auditState.fixture);
      return;
    }
    if (auditState.kind !== "loaded" || !chatID) {
      finishAction(record.id, scope);
      return;
    }

    void undoAuditRecord(consoleApi, chatID, record)
      .then((result) => {
        if (activeScopeRef.current !== scope) {
          return;
        }
        if (result.ok) {
          setRecords((currentRecords) =>
            currentRecords.map((currentRecord) =>
              currentRecord.id === record.id ? result.data : currentRecord
            )
          );
          showUndoResult(result.data);
          return;
        }

        const error = result.error;
        showFeedback(
          auditErrorMessageKey(error, "audit.errors.undoUnavailable"),
          "error",
          record,
          undefined
        );
        if (
          error.kind === "api" &&
          (error.code === "audit_conflict" || error.code === "audit_not_undoable")
        ) {
          setReloadVersion((currentVersion) => currentVersion + 1);
        }
        if (error.kind === "api" && accessRevocationCodes[error.code] === true) {
          setRecords([]);
          setAuditState({ kind: "unavailable", error });
        }
      })
      .finally(() => {
        finishAction(record.id, scope);
      });
  }

  const dataState =
    auditState.kind === "fixture"
      ? auditState.fixture.id
      : auditState.kind === "loaded"
        ? records.length === 0
          ? "empty"
          : "populated"
        : auditState.kind;

  return (
    <section
      data-record-page
      data-audit-page
      data-audit-state={dataState}
      aria-busy={auditState.kind === "loading" ? true : undefined}
      aria-labelledby="audit-title"
    >
      <header data-page-heading>
        <h1 id="audit-title">{t("audit.title")}</h1>
      </header>

      {auditState.kind === "loading" ? (
        <AuditStateCard
          id="loading"
          titleKey="audit.loading.title"
          descriptionKey="audit.loading.description"
          live="polite"
        />
      ) : null}
      {auditState.kind === "group-required" ? (
        <AuditStateCard
          id="group-required"
          titleKey="audit.groupRequired.title"
          descriptionKey="audit.groupRequired.description"
        >
          <Link to="/groups" data-slot="button" data-variant="primary" data-size="sm">
            {t("audit.groupRequired.select")}
          </Link>
        </AuditStateCard>
      ) : null}
      {auditState.kind === "no-groups" ? (
        <AuditStateCard
          id="no-groups"
          titleKey="audit.noGroups.title"
          descriptionKey="audit.noGroups.description"
        />
      ) : null}
      {auditState.kind === "unavailable" ? (
        <AuditStateCard
          id="unavailable"
          titleKey="audit.unavailable.title"
          descriptionKey={auditErrorMessageKey(
            auditState.error,
            "audit.errors.loadUnavailable"
          )}
          role="alert"
        >
          <button
            type="button"
            data-slot="button"
            data-variant="outline"
            data-size="sm"
            onClick={() => setReloadVersion((currentVersion) => currentVersion + 1)}
          >
            {t("audit.unavailable.retry")}
          </button>
        </AuditStateCard>
      ) : null}
      {(auditState.kind === "fixture" || auditState.kind === "loaded") && records.length > 0 ? (
        <AuditTable
          records={records}
          pendingActions={pendingActions}
          dateFormatter={dateFormatter}
          onUndo={undoRecord}
        />
      ) : null}
      {(auditState.kind === "fixture" || auditState.kind === "loaded") && records.length === 0 ? (
        <AuditStateCard
          id="empty"
          titleKey="audit.empty.title"
          descriptionKey="audit.empty.description"
        />
      ) : null}

      {feedback ? <AuditFeedbackNotice feedback={feedback} /> : null}
    </section>
  );
}
