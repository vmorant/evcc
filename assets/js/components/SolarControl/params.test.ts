import { describe, expect, test } from "vite-plus/test";
import {
  PARAMS,
  paramDef,
  parseParam,
  shareOfMinPower,
  smartThresholds,
  startShare,
} from "./params";
import type { SolarControlConfig } from "./types";

const cfg: SolarControlConfig = {
  mode: "off",
  strategy: "smart",
  minChargePower: 1400,
  restartCooldownMin: 60,
  socLow: 20,
  socHigh: 75,
  startShareLowSoc: 50,
  startShareHighSoc: 60,
  stopHysteresis: 50,
  baseHomeLoad: 350,
  maxStartsPerDay: 2,
  startHourFrom: 8,
  startHourTo: 17,
  startHoldMin: 10,
  stopHoldMin: 15,
  simpleStartShare: 50,
  simpleForecastShare: 55,
  simpleStopPower: 800,
  simpleShareStop: 35,
  simpleEndOfDayPower: 690,
};

describe("startShare", () => {
  test("low end", () => {
    expect(startShare(cfg, 0)).toBe(50);
    expect(startShare(cfg, 20)).toBe(50);
  });
  test("high end", () => {
    expect(startShare(cfg, 75)).toBe(60);
    expect(startShare(cfg, 100)).toBe(60);
  });
  test("interpolated", () => {
    expect(startShare(cfg, 47.5)).toBeCloseTo(55);
  });
});

describe("smartThresholds", () => {
  test("low charge level", () => {
    expect(smartThresholds(cfg, 10)).toEqual({ start: 700, stop: 350, pvStop: 700 });
  });
  test("high charge level", () => {
    expect(smartThresholds(cfg, 80)).toEqual({ start: 840, stop: 420, pvStop: 770 });
  });
});

describe("params", () => {
  test("simple start threshold", () => {
    expect(shareOfMinPower(cfg, cfg.simpleStartShare)).toBe(700);
  });
  test("every config number has a definition", () => {
    const keys = Object.keys(cfg).filter((k) => k !== "mode" && k !== "strategy");
    expect(PARAMS.map((p) => p.key).sort()).toEqual(keys.sort());
  });
  test("parse", () => {
    const hours = paramDef("startHourFrom")!;
    expect(parseParam(hours, "8.4")).toBe(8);
    expect(parseParam(hours, "")).toBeNull();
    expect(parseParam(hours, "abc")).toBeNull();
    expect(parseParam(paramDef("socLow")!, "12.5")).toBe(12.5);
  });
});
