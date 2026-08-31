import {
  challengeResults,
  type ChallengeResultDefinition
} from "../../lib/challenge";

export type TelegramMode = "approval" | "join-restrict";

export type PrerequisiteId =
  | "joinRequestsEnabled"
  | "botIsAdministrator"
  | "joinEventsDeclared";

export type Settlement = {
  result: ChallengeResultDefinition;
  count: number;
};

export type RecentApplicant = {
  userId: string;
  username: string | null;
  result: ChallengeResultDefinition;
};

export type GroupFixture = {
  id: string;
  nameKey: string;
  mode: TelegramMode;
  prerequisites: Record<PrerequisiteId, boolean>;
  applicationsLast48Hours: number;
  settlements: readonly Settlement[];
  recentApplicants: readonly RecentApplicant[];
};
export const allGroupsSelection = "all";


export const verificationPrerequisites: readonly {
  id: PrerequisiteId;
  labelKey: string;
}[] = [
  {
    id: "joinRequestsEnabled",
    labelKey: "groups.prerequisites.joinRequestsEnabled"
  },
  {
    id: "botIsAdministrator",
    labelKey: "groups.prerequisites.botIsAdministrator"
  },
  {
    id: "joinEventsDeclared",
    labelKey: "groups.prerequisites.joinEventsDeclared"
  }
];

export const modeDefinitions: Record<
  TelegramMode,
  {
    labelKey: string;
    noteKey?: string;
  }
> = {
  approval: {
    labelKey: "groups.mode.approval"
  },
  "join-restrict": {
    labelKey: "groups.mode.joinRestrict",
    noteKey: "groups.mode.joinRestrictNote"
  }
};


export const groupFixtures: readonly GroupFixture[] = [
  {
    id: "-1001163306055",
    nameKey: "groups.names.gentooZh",
    mode: "approval",
    prerequisites: {
      joinRequestsEnabled: true,
      botIsAdministrator: true,
      joinEventsDeclared: true
    },
    applicationsLast48Hours: 129,
    settlements: [
      {
        result: challengeResults.expired,
        count: 119
      },
      {
        result: challengeResults.approved,
        count: 7
      },
      {
        result: challengeResults.declinedRejected,
        count: 2
      }
    ],
    recentApplicants: [
      {
        userId: "741928306",
        username: null,
        result: challengeResults.expired
      },
      {
        userId: "528106774",
        username: "kernel_trace",
        result: challengeResults.expired
      },
      {
        userId: "890173425",
        username: null,
        result: challengeResults.approved
      }
    ]
  },
  {
    id: "-1001834029912",
    nameKey: "groups.names.archZh",
    mode: "join-restrict",
    prerequisites: {
      joinRequestsEnabled: true,
      botIsAdministrator: true,
      joinEventsDeclared: true
    },
    applicationsLast48Hours: 89,
    settlements: [
      {
        result: challengeResults.expired,
        count: 82
      },
      {
        result: challengeResults.approved,
        count: 4
      },
      {
        result: challengeResults.declinedWrongAnswer,
        count: 1
      },
      {
        result: challengeResults.declinedExternalUnmet,
        count: 1
      }
    ],
    recentApplicants: [
      {
        userId: "612005499",
        username: null,
        result: challengeResults.expired
      },
      {
        userId: "475998132",
        username: null,
        result: challengeResults.expired
      },
      {
        userId: "908116254",
        username: null,
        result: challengeResults.declinedWrongAnswer
      }
    ]
  },
  {
    id: "-1001965172048",
    nameKey: "groups.names.oldOt",
    mode: "approval",
    prerequisites: {
      joinRequestsEnabled: true,
      botIsAdministrator: true,
      joinEventsDeclared: false
    },
    applicationsLast48Hours: 1,
    settlements: [
      {
        result: challengeResults.expired,
        count: 1
      }
    ],
    recentApplicants: [
      {
        userId: "334281907",
        username: null,
        result: challengeResults.expired
      }
    ]
  }
];

export function resolveGroupSelection(value: string | null): string {
  if (value !== null && groupFixtures.some((group) => group.id === value)) {
    return value;
  }

  return allGroupsSelection;
}
