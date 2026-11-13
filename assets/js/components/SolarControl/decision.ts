import type {
  SolarControlCheck,
  SolarControlCheckKey,
  SolarControlChecksPhase,
  SolarControlStrategy,
  SolarControlValues,
} from "./types";

export type CheckState = "ok" | "fail" | "unknown";
export type CheckUnit = "W" | "%" | "none";

export function checkState(c: SolarControlCheck): CheckState {
  if (!c.known) return "unknown";
  return c.ok ? "ok" : "fail";
}

// unit of value and threshold of a check
export function checkUnit(key: SolarControlCheckKey, strategy: SolarControlStrategy): CheckUnit {
  switch (key) {
    case "surplus":
    case "sun":
    case "power":
    case "endOfDay":
      return "W";
    case "forecast":
      return strategy === "simple" ? "%" : "W";
    case "share":
      return "%";
    default:
      return "none";
  }
}

// amount missing to reach the threshold of a failing start check
export function checkGap(c: SolarControlCheck, phase: SolarControlChecksPhase): number | null {
  if (phase !== "start" || c.ok || !c.known) return null;
  if (c.value == null || c.threshold == null) return null;
  const gap = c.threshold - c.value;
  return gap > 0 ? gap : null;
}

export type VerdictKey =
  | "connect"
  | "mode"
  | "hours"
  | "dailyLimit"
  | "cooldown"
  | "hold"
  | "unknown"
  | "forecastFrom"
  | "forecastUntil"
  | "forecastNone"
  | "stopping"
  | "charging";

export interface Verdict {
  key: VerdictKey;
  /** seconds */
  remaining?: number;
  time?: Date;
  from?: number;
  to?: number;
}

const SOLAR_CHECKS: SolarControlCheckKey[] = ["surplus", "forecast", "sun"];

function parseTime(time: string | null | undefined): Date | null {
  if (!time) return null;
  const d = new Date(time);
  return Number.isNaN(d.getTime()) ? null : d;
}

function forecastVerdict(values: SolarControlValues, now: Date): Verdict {
  const start = parseTime(values.forecastWindowStart);
  const end = parseTime(values.forecastWindowEnd);
  if (start && start > now) return { key: "forecastFrom", time: start };
  if (end && end > now) return { key: "forecastUntil", time: end };
  return { key: "forecastNone" };
}

// hour of day as clock time, the end of day 24 as 24:00 instead of 00:00
export function formatHour(hour: number, fmt: (d: Date) => string): string {
  const d = new Date();
  d.setHours(hour % 24, 0, 0, 0);
  const text = fmt(d);
  return hour >= 24 ? text.replace(/^0?0(?=\D)/, "24") : text;
}

// whether and when charging will start or stop, decided by the first failing check
export function verdict(
  checks: SolarControlCheck[],
  phase: SolarControlChecksPhase,
  values: SolarControlValues,
  now: Date
): Verdict | null {
  if (phase === "stop") {
    const stopping = checks.find((c) => !c.ok && (c.remaining ?? 0) > 0);
    if (stopping) return { key: "stopping", remaining: stopping.remaining };
    return { key: "charging" };
  }
  if (phase !== "start") return null;

  const failing = checks.filter((c) => checkState(c) !== "ok");
  if (!failing.length) return null;

  // the start delay alone only postpones the start
  const first = failing.find((c) => c.key !== "hold");
  if (!first) {
    const hold = failing[0];
    return hold?.remaining ? { key: "hold", remaining: hold.remaining } : null;
  }

  switch (first.key) {
    case "connected":
      return { key: "connect" };
    case "mode":
      return { key: "mode" };
    case "hours":
      return { key: "hours", from: first.value, to: first.threshold };
    case "dailyLimit":
      return { key: "dailyLimit" };
    case "cooldown":
      return { key: "cooldown", remaining: first.remaining };
    default:
      if (!first.known) return { key: "unknown" };
      if (SOLAR_CHECKS.includes(first.key)) return forecastVerdict(values, now);
      return null;
  }
}
