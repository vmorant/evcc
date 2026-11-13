import { mount, config as testConfig } from "@vue/test-utils";
import { describe, expect, test } from "vite-plus/test";
import StatusCard from "./StatusCard.vue";
import type { SolarControlConfig, SolarControlStatus } from "./types";
import en from "../../../../i18n/en.json";

const lookup = (key: string): string | undefined => {
  const v = key.split(".").reduce<any>((o, k) => o?.[k], en);
  return typeof v === "string" ? v : undefined;
};
const t = (key: string, params: Record<string, unknown> = {}) =>
  (lookup(key) ?? key).replace(/\{(\w+)\}/g, (_, k) => String(params[k]));
testConfig.global.mocks["$t"] = t;
testConfig.global.mocks["$te"] = (key: string) => lookup(key) !== undefined;
testConfig.global.mocks["$i18n"] = { locale: "de-DE" };

const config: SolarControlConfig = {
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
  startHourTo: 24,
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
  reason: { code: "ready" },
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
    forecastEnergyRestOfDay: null,
    sunEnd: null,
    sessionEnergy: null,
    sessionSolarPercentage: null,
    forecastWindowStart: null,
    forecastWindowEnd: null,
  },
  valid: { surplusAvg15: true, pvAvg15: true, pvAvg30: true, solarShareAvg15: true },
  startsToday: 0,
  maxStartsPerDay: 2,
  cooldownRemaining: 0,
  startHoldRemaining: null,
  stopHoldRemaining: null,
  sunOk: true,
  lastStartAt: null,
  lastStopAt: null,
  events: [],
  checks: [
    { key: "connected", ok: true, known: true },
    { key: "hours", ok: true, known: true, value: 8, threshold: 24 },
    { key: "surplus", ok: false, known: true, value: 620, threshold: 789 },
  ],
  checksPhase: "start",
};

const setup = (s: Partial<SolarControlStatus> = {}) =>
  mount(StatusCard, { props: { config, status: { ...status, ...s } } });

const reasonText = (wrapper: ReturnType<typeof setup>) =>
  wrapper.get("[data-testid=solar-control-reason]").text();

describe("StatusCard", () => {
  test("ready starts now without remaining", () => {
    expect(reasonText(setup())).toBe("Conditions met, starting now.");
    expect(reasonText(setup({ reason: { code: "ready", remaining: 0 } }))).toBe(
      "Conditions met, starting now."
    );
    expect(reasonText(setup({ reason: { code: "ready", remaining: 240 } }))).toContain("starts in");
  });

  test("end of day as 24:00", () => {
    const wrapper = setup({ reason: { code: "outsideHours", from: 8, to: 24 } });
    expect(reasonText(wrapper)).toBe("Outside the start window from 08:00 to 24:00.");
    expect(wrapper.get("[data-testid=solar-control-checks]").text()).toContain("08:00–24:00");
  });

  test("selectors follow the live status", () => {
    const wrapper = setup({ strategy: "simple" });
    expect(wrapper.get("[data-testid=solarControlMode-shadow]").attributes("aria-checked")).toBe(
      "true"
    );
    expect(
      wrapper.get("[data-testid=solarControlStrategy-simple]").attributes("aria-checked")
    ).toBe("true");
  });

  test("checks with gap and verdict", () => {
    const wrapper = setup();
    const checks = wrapper.get("[data-testid=solar-control-checks]").text();
    expect(checks).toContain("Average surplus (15 min)");
    expect(checks).toContain("620 / 789 W");
    expect(checks).toContain("169 W missing");
    expect(wrapper.get("[data-testid=solar-control-verdict]").text()).toBe(
      "Forecast doesn't reach the threshold for the rest of today."
    );
  });
});
