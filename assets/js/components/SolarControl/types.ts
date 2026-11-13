// mirrors core/solarcontrol (config.go, types.go)

export type SolarControlMode = "off" | "shadow" | "active";
export type SolarControlStrategy = "smart" | "simple";

export interface SolarControlConfig {
  mode: SolarControlMode;
  strategy: SolarControlStrategy;
  minChargePower: number;
  restartCooldownMin: number;
  socLow: number;
  socHigh: number;
  startShareLowSoc: number;
  startShareHighSoc: number;
  stopHysteresis: number;
  baseHomeLoad: number;
  maxStartsPerDay: number;
  startHourFrom: number;
  startHourTo: number;
  startHoldMin: number;
  stopHoldMin: number;
  simpleStartShare: number;
  simpleForecastShare: number;
  simpleStopPower: number;
  simpleShareStop: number;
  simpleEndOfDayPower: number;
}

export type SolarControlConfigKey = Exclude<keyof SolarControlConfig, "mode" | "strategy">;

export type SolarControlReasonCode =
  | "disabled"
  | "notConnected"
  | "charging"
  | "manualNow"
  | "manualOff"
  | "manualOnce"
  | "dailyLimit"
  | "cooldown"
  | "outsideHours"
  | "warmup"
  | "socUnknown"
  | "surplusLow"
  | "forecastLow"
  | "forecastMissing"
  | "ready";

export interface SolarControlReason {
  code: SolarControlReasonCode;
  value?: number;
  threshold?: number;
  /** seconds */
  remaining?: number;
  count?: number;
  max?: number;
  from?: number;
  to?: number;
}

export interface SolarControlValues {
  surplus: number;
  surplusAvg15: number;
  pvAvg15: number;
  pvAvg30: number;
  solarShareAvg15: number;
  startThreshold: number | null;
  stopThreshold: number | null;
  pvStopThreshold: number | null;
  forecastSurplus2h: number | null;
  forecastCoverage: number | null;
  soc: number | null;
  sunDeclining: boolean | null;
}

export interface SolarControlValid {
  surplusAvg15: boolean;
  pvAvg15: boolean;
  pvAvg30: boolean;
  solarShareAvg15: boolean;
}

export type SolarControlEventType =
  | "start"
  | "stop"
  | "resume"
  | "disconnect"
  | "shadow"
  | "external";

export type SolarControlAction = "start" | "resume" | "stop" | "disconnect";

export type SolarControlStopRule = "sunLow" | "power" | "share" | "endOfDay" | "manualOff";

export interface SolarControlEvent {
  time: string;
  type: SolarControlEventType;
  strategy: SolarControlStrategy;
  action?: SolarControlAction;
  rule?: SolarControlStopRule;
  surplus?: number;
  pv?: number;
  threshold?: number;
  /** kWh */
  energy?: number;
  solarPercentage?: number;
  resetMode?: boolean;
  resetAlways?: boolean;
}

export interface SolarControlStatus {
  mode: SolarControlMode;
  strategy: SolarControlStrategy;
  reason: SolarControlReason;
  values: SolarControlValues;
  valid: SolarControlValid;
  startsToday: number;
  maxStartsPerDay: number;
  /** seconds */
  cooldownRemaining: number;
  /** seconds, null if the start condition does not hold */
  startHoldRemaining: number | null;
  /** seconds, null if no stop timer runs */
  stopHoldRemaining: number | null;
  sunOk: boolean;
  lastStartAt: string | null;
  lastStopAt: string | null;
  /** most recent first */
  events: SolarControlEvent[];
}

export interface SolarControlResponse {
  config: SolarControlConfig;
  status: SolarControlStatus;
}

/** 400 response body of POST /api/solarcontrol/config */
export interface SolarControlValidationError {
  field: string;
  error: string;
}
