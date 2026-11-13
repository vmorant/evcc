import StatusCard from "./StatusCard.vue";
import type { Meta, StoryFn } from "@storybook/vue3";
import type { SolarControlConfig, SolarControlStatus } from "./types";

export default {
  title: "SolarControl/StatusCard",
  component: StatusCard,
} as Meta<typeof StatusCard>;

const Template: StoryFn<typeof StatusCard> = (args) => ({
  components: { StatusCard },
  setup() {
    return { args };
  },
  template: '<StatusCard v-bind="args" />',
});

const config: SolarControlConfig = {
  mode: "shadow",
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

const status: SolarControlStatus = {
  mode: "shadow",
  strategy: "smart",
  reason: { code: "surplusLow", value: 620, threshold: 789 },
  values: {
    surplus: 1100,
    surplusAvg15: 980,
    pvAvg15: 1650,
    pvAvg30: 1500,
    solarShareAvg15: 0,
    startThreshold: 770,
    stopThreshold: 385,
    pvStopThreshold: 735,
    forecastSurplus2h: 640,
    forecastCoverage: 72,
    soc: 47.5,
    sunDeclining: false,
    pv: 1650,
    home: 550,
    chargePower: null,
    forecastEnergyRestOfDay: 6200,
    sunEnd: "2026-11-13T16:12:00Z",
    sessionEnergy: null,
    sessionSolarPercentage: null,
    forecastWindowStart: "2026-11-13T10:30:00Z",
    forecastWindowEnd: "2026-11-13T13:00:00Z",
  },
  valid: { surplusAvg15: true, pvAvg15: true, pvAvg30: false, solarShareAvg15: false },
  startsToday: 1,
  maxStartsPerDay: 2,
  cooldownRemaining: 0,
  startHoldRemaining: 420,
  stopHoldRemaining: null,
  sunOk: true,
  lastStartAt: "2026-11-13T08:41:00Z",
  lastStopAt: "2026-11-13T10:11:00Z",
  events: [],
  checks: [
    { key: "connected", ok: true, known: true },
    { key: "mode", ok: true, known: true },
    { key: "hours", ok: true, known: true, value: 8, threshold: 17 },
    { key: "surplus", ok: false, known: true, value: 620, threshold: 789 },
    { key: "hold", ok: false, known: true },
    { key: "forecast", ok: false, known: true, value: 640, threshold: 789 },
    { key: "cooldown", ok: true, known: true },
    { key: "dailyLimit", ok: true, known: true, value: 1, threshold: 2 },
  ],
  checksPhase: "start",
};

export const SmartShadow = Template.bind({});
SmartShadow.args = { config, status };

export const SmartWarmup = Template.bind({});
SmartWarmup.args = {
  config: { ...config, mode: "active" },
  status: {
    ...status,
    mode: "active",
    reason: { code: "warmup" },
    checks: status.checks.map((c) =>
      c.key === "surplus" ? { key: "surplus", ok: false, known: false } : c
    ),
    valid: { surplusAvg15: false, pvAvg15: false, pvAvg30: false, solarShareAvg15: false },
    values: { ...status.values, soc: null, startThreshold: null, pvStopThreshold: null },
    startHoldRemaining: null,
  },
};

export const SimpleCharging = Template.bind({});
SimpleCharging.args = {
  config: { ...config, mode: "active", strategy: "simple" },
  status: {
    ...status,
    mode: "active",
    strategy: "simple",
    reason: { code: "charging", value: 1500 },
    valid: { surplusAvg15: true, pvAvg15: true, pvAvg30: true, solarShareAvg15: true },
    startHoldRemaining: null,
    values: { ...status.values, chargePower: 1400, sessionEnergy: 3.2, sessionSolarPercentage: 78 },
    checks: [
      { key: "power", ok: true, known: true, value: 1500, threshold: 800 },
      { key: "share", ok: true, known: true, value: 78, threshold: 35 },
      { key: "endOfDay", ok: true, known: true, value: 1500, threshold: 690 },
      { key: "sunDeclining", ok: true, known: true },
    ],
    checksPhase: "stop",
  },
};

export const SmartStopping = Template.bind({});
SmartStopping.args = {
  config: { ...config, mode: "active" },
  status: {
    ...status,
    mode: "active",
    reason: { code: "charging", value: 450, threshold: 735, remaining: 420 },
    values: { ...status.values, chargePower: 1400, sessionEnergy: 5.1, sessionSolarPercentage: 64 },
    checks: [
      { key: "sun", ok: false, known: true, value: 450, threshold: 735 },
      { key: "stopHold", ok: false, known: true, remaining: 420 },
    ],
    checksPhase: "stop",
  },
};

export const SmartStartDelay = Template.bind({});
SmartStartDelay.args = {
  config,
  status: {
    ...status,
    reason: { code: "ready", remaining: 240 },
    checks: status.checks.map((c) => {
      if (c.key === "hold") return { ...c, remaining: 240 };
      return { ...c, ok: true };
    }),
  },
};

export const NoStatus = Template.bind({});
NoStatus.args = { config: { ...config, mode: "off" }, status: null };
