export function browserTimeZone(): string {
  try {
    const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    return timeZone.length > 0 ? timeZone : "UTC";
  } catch {
    return "UTC";
  }
}

function calendarDateInTimeZone(timeZone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit"
  }).formatToParts();
  const values: Record<string, string> = {};

  for (const part of parts) {
    if (part.type === "year" || part.type === "month" || part.type === "day") {
      values[part.type] = part.value;
    }
  }

  return values.year && values.month && values.day
    ? `${values.year}-${values.month}-${values.day}`
    : new Date().toISOString().slice(0, 10);
}

function shiftCalendarDate(date: string, days: number): string {
  const shifted = new Date(`${date}T00:00:00Z`);
  shifted.setUTCDate(shifted.getUTCDate() + days);
  return shifted.toISOString().slice(0, 10);
}

export function defaultStatsQuery(timeZone = browserTimeZone()) {
  const today = calendarDateInTimeZone(timeZone);
  return {
    from: shiftCalendarDate(today, -6),
    to: shiftCalendarDate(today, 1),
    timezone: timeZone
  };
}
