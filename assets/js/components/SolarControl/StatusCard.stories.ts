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
  reason: { code: "ready", remaining: 420 },
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
  },
};

export const NoStatus = Template.bind({});
NoStatus.args = { config: { ...config, mode: "off" }, status: null };
