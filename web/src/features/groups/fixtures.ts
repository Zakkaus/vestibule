export type TelegramMode = "approval" | "join-restrict";

export type PrerequisiteId =
  | "joinRequestsEnabled"
  | "botIsAdministrator"
  | "joinEventsDeclared";

export type SettlementReason = "timeout" | "approved" | "rejected";

export type RecentApplicant = {
  userId: string;
  username: string | null;
  settlement: SettlementReason;
};

export type GroupFixture = {
  id: string;
  nameKey: string;
  mode: TelegramMode;
  prerequisites: Record<PrerequisiteId, boolean>;
  applicationsLast48Hours: number;
  settlements: Readonly<Record<SettlementReason, number>>;
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

export const settlementDefinitions: readonly {
  id: SettlementReason;
  labelKey: string;
}[] = [
  {
    id: "timeout",
    labelKey: "groups.settlement.timeout"
  },
  {
    id: "approved",
    labelKey: "groups.settlement.approved"
  },
  {
    id: "rejected",
    labelKey: "groups.settlement.rejected"
  }
];

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
    settlements: {
      timeout: 119,
      approved: 7,
      rejected: 2
    },
    recentApplicants: [
      {
        userId: "741928306",
        username: null,
        settlement: "timeout"
      },
      {
        userId: "528106774",
        username: "kernel_trace",
        settlement: "timeout"
      },
      {
        userId: "890173425",
        username: null,
        settlement: "approved"
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
    settlements: {
      timeout: 82,
      approved: 4,
      rejected: 2
    },
    recentApplicants: [
      {
        userId: "612005499",
        username: null,
        settlement: "timeout"
      },
      {
        userId: "475998132",
        username: null,
        settlement: "timeout"
      },
      {
        userId: "908116254",
        username: null,
        settlement: "rejected"
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
    settlements: {
      timeout: 1,
      approved: 0,
      rejected: 0
    },
    recentApplicants: [
      {
        userId: "334281907",
        username: null,
        settlement: "timeout"
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
