import {
  nonEmptyStringFromPayload,
  objectFromPayload
} from "./api";

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

const settledChallengeResults = {
  pending: challengeResults.pending,
  approved: challengeResults.approved,
  banned: challengeResults.banned,
  expired: challengeResults.expired,
  superseded: challengeResults.superseded
} as const satisfies Readonly<
  Record<Exclude<ChallengeState, "declined">, ChallengeResultDefinition>
>;

const declinedChallengeResults = {
  wrong_answer: challengeResults.declinedWrongAnswer,
  rejected: challengeResults.declinedRejected,
  external_unmet: challengeResults.declinedExternalUnmet
} as const satisfies Readonly<Record<DeclineReason, ChallengeResultDefinition>>;

export function challengeResultFromPayload(
  value: unknown
): ChallengeResultDefinition | undefined {
  const result = objectFromPayload(value);
  const state = result ? nonEmptyStringFromPayload(result.state) : undefined;

  if (!result || !state || !("reason" in result)) {
    return undefined;
  }

  if (state === "declined") {
    const reason = result.reason;
    return typeof reason === "string" && reason in declinedChallengeResults
      ? declinedChallengeResults[reason as DeclineReason]
      : undefined;
  }

  return result.reason === null && state in settledChallengeResults
    ? settledChallengeResults[state as Exclude<ChallengeState, "declined">]
    : undefined;
}
