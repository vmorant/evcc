import { describe, expect, test } from "vite-plus/test";
import { checkGap, checkState, checkUnit, formatHour, verdict } from "./decision";
import type { SolarControlCheck, SolarControlValues } from "./types";

const values: SolarControlValues = {
  surplus: 0,
  surplusAvg15: 0,
  pvAvg15: 0,
  pvAvg30: 0,
  solarShareAvg15: 0,
  startThreshold: null,
  stopThreshold: null,
  pvStopThreshold: null,
  forecastSurplus2h: null,
  forecastCoverage: null,
  soc: null,
  sunDeclining: null,
  pv: null,
  home: null,
  chargePower: null,
  forecastEnergyRestOfDay: null,
  sunEnd: null,
  sessionEnergy: null,
  sessionSolarPercentage: null,
  forecastWindowStart: null,
  forecastWindowEnd: null,
};

const now = new Date("2026-11-13T10:00:00Z");

const ok = (key: SolarControlCheck["key"]): SolarControlCheck => ({ key, ok: true, known: true });
const fail = (
  key: SolarControlCheck["key"],
  extra: Partial<SolarControlCheck> = {}
): SolarControlCheck => ({ key, ok: false, known: true, ...extra });

describe("checks", () => {
  test("state", () => {
    expect(checkState(ok("connected"))).toBe("ok");
    expect(checkState(fail("surplus"))).toBe("fail");
    expect(checkState({ key: "surplus", ok: false, known: false })).toBe("unknown");
  });

  test("unit", () => {
    expect(checkUnit("forecast", "smart")).toBe("W");
    expect(checkUnit("forecast", "simple")).toBe("%");
    expect(checkUnit("hours", "smart")).toBe("none");
  });

  test("gap", () => {
    const c = fail("surplus", { value: 620, threshold: 789 });
    expect(checkGap(c, "start")).toBe(169);
    expect(checkGap(c, "stop")).toBeNull();
    expect(checkGap(ok("surplus"), "start")).toBeNull();
  });
});

describe("formatHour", () => {
  const fmt = (d: Date) => `${String(d.getHours()).padStart(2, "0")}:00`;
  const fmt12 = (d: Date) => `${d.getHours() % 12 || 12}:00 ${d.getHours() < 12 ? "AM" : "PM"}`;

  test("24 hour clock", () => {
    expect(formatHour(8, fmt)).toBe("08:00");
    expect(formatHour(0, fmt)).toBe("00:00");
    expect(formatHour(24, fmt)).toBe("24:00");
  });

  test("12 hour clock keeps midnight", () => {
    expect(formatHour(24, fmt12)).toBe("12:00 AM");
  });
});

describe("verdict", () => {
  test("no phase", () => {
    expect(verdict([], "", values, now)).toBeNull();
  });

  test("all met", () => {
    expect(verdict([ok("connected"), ok("surplus")], "start", values, now)).toBeNull();
  });

  test("first failing check decides", () => {
    const checks = [ok("connected"), fail("mode"), fail("hours", { value: 8, threshold: 17 })];
    expect(verdict(checks, "start", values, now)).toEqual({ key: "mode" });
    expect(verdict(checks.slice(2), "start", values, now)).toEqual({
      key: "hours",
      from: 8,
      to: 17,
    });
  });

  test("only start delay pending", () => {
    const checks = [ok("surplus"), fail("hold", { remaining: 240 }), ok("forecast")];
    expect(verdict(checks, "start", values, now)).toEqual({ key: "hold", remaining: 240 });
  });

  test("start delay with another failing check", () => {
    const checks = [ok("surplus"), fail("hold", { remaining: 240 }), fail("cooldown")];
    expect(verdict(checks, "start", values, now)?.key).toBe("cooldown");
  });

  test("unknown average", () => {
    const checks = [{ key: "surplus", ok: false, known: false } as SolarControlCheck];
    expect(verdict(checks, "start", values, now)).toEqual({ key: "unknown" });
  });

  test("forecast window", () => {
    const checks = [fail("surplus", { value: 100, threshold: 700 })];
    const at = (start: string | null, end: string | null) =>
      verdict(
        checks,
        "start",
        { ...values, forecastWindowStart: start, forecastWindowEnd: end },
        now
      );

    expect(at("2026-11-13T11:30:00Z", "2026-11-13T14:00:00Z")).toEqual({
      key: "forecastFrom",
      time: new Date("2026-11-13T11:30:00Z"),
    });
    expect(at("2026-11-13T09:00:00Z", "2026-11-13T14:00:00Z")).toEqual({
      key: "forecastUntil",
      time: new Date("2026-11-13T14:00:00Z"),
    });
    expect(at("2026-11-13T08:00:00Z", "2026-11-13T09:00:00Z")).toEqual({ key: "forecastNone" });
    expect(at(null, null)).toEqual({ key: "forecastNone" });
  });

  test("stop phase", () => {
    expect(verdict([ok("sun"), ok("stopHold")], "stop", values, now)).toEqual({
      key: "charging",
    });
    const stopping = [fail("sun"), fail("stopHold", { remaining: 420 })];
    expect(verdict(stopping, "stop", values, now)).toEqual({ key: "stopping", remaining: 420 });
  });
});
