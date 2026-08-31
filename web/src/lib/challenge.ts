export const challengeStates = [
  "pending",
  "approved",
  "declined",
  "banned",
  "expired",
  "superseded"
] as const;

export type ChallengeState = (typeof challengeStates)[number];

export const declineReasons = ["wrong_answer", "rejected", "external_unmet"] as const;

export type DeclineReason = (typeof declineReasons)[number];

export type ChallengeResult =
  | {
      readonly state: "declined";
      readonly reason: DeclineReason;
    }
  | {
      readonly state: Exclude<ChallengeState, "declined">;
      readonly reason: null;
    };

export type ChallengeResultId =
  | "pending"
  | "approved"
  | "declinedWrongAnswer"
  | "declinedRejected"
  | "declinedExternalUnmet"
  | "banned"
  | "expired"
  | "superseded";

export type ChallengeResultDefinition = ChallengeResult & {
  readonly id: ChallengeResultId;
  readonly labelKey: string;
  readonly tone: "ok" | "pending" | "error" | "neutral";
};

export const challengeResults = {
  pending: {
    id: "pending",
    state: "pending",
    reason: null,
    labelKey: "challenge.state.pending",
    tone: "pending"
  },
  approved: {
    id: "approved",
    state: "approved",
    reason: null,
    labelKey: "challenge.state.approved",
    tone: "ok"
  },
  declinedWrongAnswer: {
    id: "declinedWrongAnswer",
    state: "declined",
    reason: "wrong_answer",
    labelKey: "challenge.reason.wrongAnswer",
    tone: "error"
  },
  declinedRejected: {
    id: "declinedRejected",
    state: "declined",
    reason: "rejected",
    labelKey: "challenge.reason.rejected",
    tone: "error"
  },
  declinedExternalUnmet: {
    id: "declinedExternalUnmet",
    state: "declined",
    reason: "external_unmet",
    labelKey: "challenge.reason.externalUnmet",
    tone: "error"
  },
  banned: {
    id: "banned",
    state: "banned",
    reason: null,
    labelKey: "challenge.state.banned",
    tone: "error"
  },
  expired: {
    id: "expired",
    state: "expired",
    reason: null,
    labelKey: "challenge.state.expired",
    tone: "neutral"
  },
  superseded: {
    id: "superseded",
    state: "superseded",
    reason: null,
    labelKey: "challenge.state.superseded",
    tone: "neutral"
  }
} as const satisfies Record<ChallengeResultId, ChallengeResultDefinition>;
