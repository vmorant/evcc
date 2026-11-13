import type { SolarControlConfig, SolarControlConfigKey } from "./types";

export type ParamGroup = "general" | "smart" | "simple";
export type ParamUnit = "W" | "%" | "min" | "h" | "";

export interface ParamDef {
  key: SolarControlConfigKey;
  group: ParamGroup;
  unit: ParamUnit;
  min: number;
  max: number;
  step: number;
  integer: boolean;
}

// ranges mirror Config.Validate in core/solarcontrol/config.go
export const PARAMS: ParamDef[] = [
  {
    key: "minChargePower",
    group: "general",
    unit: "W",
    min: 500,
    max: 11000,
    step: 50,
    integer: false,
  },
  {
    key: "restartCooldownMin",
    group: "general",
    unit: "min",
    min: 0,
    max: 120,
    step: 5,
    integer: true,
  },
  { key: "socLow", group: "smart", unit: "%", min: 0, max: 100, step: 5, integer: false },
  { key: "socHigh", group: "smart", unit: "%", min: 0, max: 100, step: 5, integer: false },
  {
    key: "startShareLowSoc",
    group: "smart",
    unit: "%",
    min: 10,
    max: 150,
    step: 5,
    integer: false,
  },
  {
    key: "startShareHighSoc",
    group: "smart",
    unit: "%",
    min: 10,
    max: 150,
    step: 5,
    integer: false,
  },
  { key: "stopHysteresis", group: "smart", unit: "%", min: 10, max: 100, step: 5, integer: false },
  { key: "baseHomeLoad", group: "smart", unit: "W", min: 100, max: 1000, step: 50, integer: false },
  { key: "maxStartsPerDay", group: "smart", unit: "", min: 1, max: 10, step: 1, integer: true },
  { key: "startHourFrom", group: "smart", unit: "h", min: 0, max: 24, step: 1, integer: true },
  { key: "startHourTo", group: "smart", unit: "h", min: 0, max: 24, step: 1, integer: true },
  { key: "startHoldMin", group: "smart", unit: "min", min: 0, max: 60, step: 1, integer: true },
  { key: "stopHoldMin", group: "smart", unit: "min", min: 0, max: 60, step: 1, integer: true },
  {
    key: "simpleStartShare",
    group: "simple",
    unit: "%",
    min: 10,
    max: 100,
    step: 5,
    integer: false,
  },
  {
    key: "simpleForecastShare",
    group: "simple",
    unit: "%",
    min: 10,
    max: 100,
    step: 5,
    integer: false,
  },
  {
    key: "simpleStopPower",
    group: "simple",
    unit: "W",
    min: 0,
    max: 2000,
    step: 50,
    integer: false,
  },
  { key: "simpleShareStop", group: "simple", unit: "%", min: 0, max: 100, step: 5, integer: false },
  {
    key: "simpleEndOfDayPower",
    group: "simple",
    unit: "W",
    min: 0,
    max: 2000,
    step: 10,
    integer: false,
  },
];

export function paramDef(key: string): ParamDef | undefined {
  return PARAMS.find((p) => p.key === key);
}

// parses an input value, null if it is not a finite number
export function parseParam(def: ParamDef, raw: string): number | null {
  if (raw.trim() === "") return null;
  const value = Number(raw);
  if (!Number.isFinite(value)) return null;
  return def.integer ? Math.round(value) : value;
}

// start share in % of minChargePower for a charge level, like the backend share(soc)
export function startShare(cfg: SolarControlConfig, soc: number): number {
  const { socLow, socHigh, startShareLowSoc, startShareHighSoc } = cfg;
  if (soc <= socLow) return startShareLowSoc;
  if (soc >= socHigh) return startShareHighSoc;
  const t = (soc - socLow) / (socHigh - socLow);
  return startShareLowSoc + t * (startShareHighSoc - startShareLowSoc);
}

export interface SmartThresholds {
  start: number;
  stop: number;
  pvStop: number;
}

// derived smart thresholds in W for a charge level
export function smartThresholds(cfg: SolarControlConfig, soc: number): SmartThresholds {
  const start = (cfg.minChargePower * startShare(cfg, soc)) / 100;
  const stop = (start * cfg.stopHysteresis) / 100;
  return { start, stop, pvStop: stop + cfg.baseHomeLoad };
}

// share of minChargePower in W
export function shareOfMinPower(cfg: SolarControlConfig, share: number): number {
  return (cfg.minChargePower * share) / 100;
}
