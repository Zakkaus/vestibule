import {
  challengeResults,
  type ChallengeResultDefinition
} from "../../lib/challenge";
import type { QueueRecord } from "./api";

export type QueueFixtureRecord = QueueRecord & {
  readonly simulatedFailureAction?: "release";
};

export type QueueFilter = {
  groupKey: string;
  groupLabelKey?: string;
  result: ChallengeResultDefinition;
};

export type QueueFixture = {
  id: string;
  records: readonly QueueFixtureRecord[];
  filter?: QueueFilter;
};

const defaultQueueFixture: QueueFixture = {
  id: "populated",
  records: [
    {
      id: "challenge-42",
      user: "@someone",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.approved,
      occurredAt: "2026-08-31T14:02:00+08:00",
      expiresAt: "2026-08-31T14:32:00+08:00"
    },
    {
      id: "challenge-43",
      user: "@another",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.pending,
      occurredAt: "2026-08-31T14:09:00+08:00",
      expiresAt: "2026-08-31T14:11:41+08:00",
      remainingSeconds: 161
    },
    {
      id: "challenge-50",
      user: "@retry_release",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.pending,
      occurredAt: "2026-08-31T14:08:00+08:00",
      expiresAt: "2026-08-31T14:11:47+08:00",
      remainingSeconds: 219,
      simulatedFailureAction: "release"
    },
    {
      id: "challenge-44",
      user: "@spam_ad_01",
      groupKey: "-1001834029912",
      groupLabelKey: "groups.names.archZh",
      result: challengeResults.banned,
      occurredAt: "2026-08-31T13:47:00+08:00",
      expiresAt: "2026-08-31T14:17:00+08:00"
    },
    {
      id: "challenge-45",
      user: "@lurker",
      groupKey: "-1001965172048",
      groupLabelKey: "groups.names.oldOt",
      result: challengeResults.expired,
      occurredAt: "2026-08-30T21:15:00+08:00",
      expiresAt: "2026-08-30T21:45:00+08:00"
    },
    {
      id: "challenge-46",
      user: "@wrong_answer",
      groupKey: "-1001834029912",
      groupLabelKey: "groups.names.archZh",
      result: challengeResults.declinedWrongAnswer,
      occurredAt: "2026-08-30T20:41:00+08:00",
      expiresAt: "2026-08-30T21:11:00+08:00"
    },
    {
      id: "challenge-47",
      user: "@policy_veto",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.declinedRejected,
      occurredAt: "2026-08-30T19:36:00+08:00",
      expiresAt: "2026-08-30T20:06:00+08:00"
    },
    {
      id: "challenge-48",
      user: "@external_check",
      groupKey: "-1001965172048",
      groupLabelKey: "groups.names.oldOt",
      result: challengeResults.declinedExternalUnmet,
      occurredAt: "2026-08-30T18:22:00+08:00",
      expiresAt: "2026-08-30T18:52:00+08:00"
    },
    {
      id: "challenge-49",
      user: "@reapplied_user",
      groupKey: "-1001163306055",
      groupLabelKey: "groups.names.gentooZh",
      result: challengeResults.superseded,
      occurredAt: "2026-08-30T17:08:00+08:00",
      expiresAt: "2026-08-30T17:38:00+08:00"
    }
  ]
};

export const queueFixtures: readonly QueueFixture[] = [
  defaultQueueFixture,
  {
    id: "empty",
    records: []
  },
  {
    id: "filtered-empty",
    records: [],
    filter: {
      groupKey: "-1001965172048",
      groupLabelKey: "groups.names.oldOt",
      result: challengeResults.banned
    }
  }
];

export function queueFixtureFor(fixtureId: string | null): QueueFixture {
  return queueFixtures.find((fixture) => fixture.id === fixtureId) ?? defaultQueueFixture;
}
