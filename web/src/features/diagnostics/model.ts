import type { DiagnosticsDatabaseWrites, DiagnosticsProblemStreak, DiagnosticsRejections } from "./api";

export type DiagnosticsFormatters = Readonly<{
  date: Intl.DateTimeFormat;
  number: Intl.NumberFormat;
  rate: Intl.NumberFormat;
}>;

export const streakStates = ["clear", "within", "exceeded"] as const;

export type StreakState = (typeof streakStates)[number];

export const writeRateStates = ["no-writes", "within", "exceeded"] as const;

export type WriteRateState = (typeof writeRateStates)[number];

export const rejectionsStates = ["unavailable", "none", "listed"] as const;

export type RejectionsState = (typeof rejectionsStates)[number];

export const durationUnits = ["seconds", "minutes", "hours", "minutesSeconds"] as const;

export type DurationUnit = (typeof durationUnits)[number];

export type DurationParts = Readonly<{
  unit: DurationUnit;
  hours: number;
  minutes: number;
  seconds: number;
}>;

const secondsPerMinute = 60;
const secondsPerHour = 60 * secondsPerMinute;

// The span of the current unbroken run is the reading; the number of failures in
// it is not. One failure spans zero seconds, so it can never read as sustained.
export function streakState(streak: DiagnosticsProblemStreak): StreakState {
  if (streak.exceedsThreshold) {
    return "exceeded";
  }
  return streak.firstProblemAt === null ? "clear" : "within";
}

// A rate over no writes at all is zero, which reads as a healthy zero percent.
// The empty window is its own state so nobody reads it as one.
export function writeRateState(writes: DiagnosticsDatabaseWrites): WriteRateState {
  if (writes.totalWrites === 0) {
    return "no-writes";
  }
  return writes.exceedsOnePercent ? "exceeded" : "within";
}

export function rejectionsState(rejections: DiagnosticsRejections): RejectionsState {
  if (!rejections.sourceAvailable) {
    return "unavailable";
  }
  return rejections.byReason.length === 0 ? "none" : "listed";
}

export function durationParts(totalSeconds: number): DurationParts {
  const whole = Math.max(0, Math.round(totalSeconds));
  const seconds = whole % secondsPerMinute;

  if (whole < secondsPerMinute) {
    return { unit: "seconds", hours: 0, minutes: 0, seconds: whole };
  }
  if (whole % secondsPerHour === 0) {
    return { unit: "hours", hours: whole / secondsPerHour, minutes: 0, seconds: 0 };
  }
  if (seconds === 0) {
    return { unit: "minutes", hours: 0, minutes: whole / secondsPerMinute, seconds: 0 };
  }
  return {
    unit: "minutesSeconds",
    hours: 0,
    minutes: Math.floor(whole / secondsPerMinute),
    seconds
  };
}
