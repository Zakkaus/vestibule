import {
  challengeResults,
  type ChallengeResultDefinition,
  type ChallengeResultId
} from "../../lib/challenge";

export type QueueActionId = "release" | "revoke" | "details";

export type QueueRecord = {
  id: string;
  user: string;
  groupKey: string;
  result: ChallengeResultDefinition;
  occurredAt: string;
  remainingSeconds?: number;
};

export type QueueFilter = {
  groupKey: string;
  result: ChallengeResultDefinition;
};

export type QueueFixture = {
  id: string;
  records: readonly QueueRecord[];
  filter?: QueueFilter;
};

export const queueResultPresentations: Readonly<
  Record<
    ChallengeResultId,
    {
      action: QueueActionId | null;
      showsRemainingTime: boolean;
    }
  >
> = {
  pending: {
    action: "release",
    showsRemainingTime: true
  },
  approved: {
    action: "details",
    showsRemainingTime: false
  },
  declinedWrongAnswer: {
    action: null,
    showsRemainingTime: false
  },
  declinedRejected: {
    action: null,
    showsRemainingTime: false
  },
  declinedExternalUnmet: {
    action: null,
    showsRemainingTime: false
  },
  banned: {
    action: "revoke",
    showsRemainingTime: false
  },
  expired: {
    action: "details",
    showsRemainingTime: false
  },
  superseded: {
    action: null,
    showsRemainingTime: false
  }
};

export const queueActions: Readonly<
  Record<
    QueueActionId,
    {
      labelKey: string;
      ariaLabelKey: string;
      variant: "primary" | "ghost";
    }
  >
> = {
  release: {
    labelKey: "queue.actions.release",
    ariaLabelKey: "queue.actions.releaseFor",
    variant: "primary"
  },
  revoke: {
    labelKey: "queue.actions.revoke",
    ariaLabelKey: "queue.actions.revokeFor",
    variant: "ghost"
  },
  details: {
    labelKey: "queue.actions.details",
    ariaLabelKey: "queue.actions.detailsFor",
    variant: "ghost"
  }
};

const defaultQueueFixture: QueueFixture = {
  id: "populated",
  records: [
    {
      id: "challenge-42",
      user: "@someone",
      groupKey: "groups.names.gentooZh",
      result: challengeResults.approved,
      occurredAt: "2026-08-31T14:02:00+08:00"
    },
    {
      id: "challenge-43",
      user: "@another",
      groupKey: "groups.names.gentooZh",
      result: challengeResults.pending,
      occurredAt: "2026-08-31T14:09:00+08:00",
      remainingSeconds: 161
    },
    {
      id: "challenge-44",
      user: "@spam_ad_01",
      groupKey: "groups.names.archZh",
      result: challengeResults.banned,
      occurredAt: "2026-08-31T13:47:00+08:00"
    },
    {
      id: "challenge-45",
      user: "@lurker",
      groupKey: "groups.names.oldOt",
      result: challengeResults.expired,
      occurredAt: "2026-08-30T21:15:00+08:00"
    },
    {
      id: "challenge-46",
      user: "@wrong_answer",
      groupKey: "groups.names.archZh",
      result: challengeResults.declinedWrongAnswer,
      occurredAt: "2026-08-30T20:41:00+08:00"
    },
    {
      id: "challenge-47",
      user: "@policy_veto",
      groupKey: "groups.names.gentooZh",
      result: challengeResults.declinedRejected,
      occurredAt: "2026-08-30T19:36:00+08:00"
    },
    {
      id: "challenge-48",
      user: "@external_check",
      groupKey: "groups.names.oldOt",
      result: challengeResults.declinedExternalUnmet,
      occurredAt: "2026-08-30T18:22:00+08:00"
    },
    {
      id: "challenge-49",
      user: "@reapplied_user",
      groupKey: "groups.names.gentooZh",
      result: challengeResults.superseded,
      occurredAt: "2026-08-30T17:08:00+08:00"
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
      groupKey: "groups.names.oldOt",
      result: challengeResults.banned
    }
  }
];

export function queueFixtureFor(fixtureId: string | null): QueueFixture {
  return queueFixtures.find((fixture) => fixture.id === fixtureId) ?? defaultQueueFixture;
}
