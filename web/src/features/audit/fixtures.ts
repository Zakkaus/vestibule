import { challengeResults } from "../../lib/challenge";
import type { AuditRecord } from "./api";

export type AuditFixtureRecord = AuditRecord & {
  readonly simulatedUndoOutcome?: "failure";
};

export type AuditFixture = Readonly<{
  id: string;
  records: readonly AuditFixtureRecord[];
}>;

const availableAuditRecord: AuditFixtureRecord = {
  id: "-1001163306055:44:audit-ban",
  user: "@spam_forwarding_account",
  groupKey: "-1001163306055",
  groupLabelKey: "groups.names.gentooZh",
  result: challengeResults.banned,
  settledAt: "2026-08-31T14:02:00+08:00",
  settledBy: "9",
  undoState: "available"
};

const defaultAuditFixture: AuditFixture = {
  id: "populated",
  records: [
    availableAuditRecord,
    {
      id: "-1001163306055:46:audit-wrong",
      user: "@wrong_answer",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.declinedWrongAnswer,
      settledAt: "2026-08-31T13:58:00+08:00",
      settledBy: "9",
      undoState: "unavailable"
    },
    {
      id: "-1001163306055:42:audit-approved",
      user: "@someone",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.approved,
      settledAt: "2026-08-31T13:41:00+08:00",
      settledBy: null,
      undoState: "unavailable"
    },
    {
      id: "-1001163306055:45:audit-expired",
      user: "@patient_applicant",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.expired,
      settledAt: "2026-08-31T13:35:00+08:00",
      settledBy: null,
      undoState: "unavailable"
    },
    {
      id: "-1001163306055:47:audit-superseded",
      user: "@reapplied_user",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.superseded,
      settledAt: "2026-08-31T13:27:00+08:00",
      settledBy: null,
      undoState: "unavailable"
    },
    {
      id: "-1001163306055:48:audit-other-actor",
      user: "@other_admin_decision",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.banned,
      settledAt: "2026-08-31T13:09:00+08:00",
      settledBy: "17",
      undoState: "unavailable"
    }
  ]
};

export const auditFixtures: readonly AuditFixture[] = [
  defaultAuditFixture,
  { id: "empty", records: [] },
  {
    id: "undo-failure",
    records: [
      {
        ...availableAuditRecord,
        id: "-1001163306055:49:audit-failure",
        user: "@undo_retry_target",
        simulatedUndoOutcome: "failure"
      }
    ]
  }
];

export function auditFixtureFor(fixtureID: string | null): AuditFixture {
  return auditFixtures.find((fixture) => fixture.id === fixtureID) ?? defaultAuditFixture;
}
