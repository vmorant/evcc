import { mount, config as testConfig } from "@vue/test-utils";
import { describe, expect, test } from "vite-plus/test";
import ParamsCard from "./ParamsCard.vue";
import type { SolarControlConfig, SolarControlValidationError } from "./types";
import en from "../../../../i18n/en.json";

const lookup = (key: string): string | undefined => {
  const v = key.split(".").reduce<any>((o, k) => o?.[k], en);
  return typeof v === "string" ? v : undefined;
};
const t = (key: string, params: Record<string, unknown> = {}) =>
  (lookup(key) ?? key).replace(/\{(\w+)\}/g, (_, k) => String(params[k]));
testConfig.global.mocks["$t"] = t;
testConfig.global.mocks["$te"] = (key: string) => lookup(key) !== undefined;
testConfig.global.mocks["$i18n"] = { locale: "en-GB" };

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
  startHourTo: 17,
  startHoldMin: 10,
  stopHoldMin: 15,
  simpleStartShare: 50,
  simpleForecastShare: 55,
  simpleStopPower: 800,
  simpleShareStop: 35,
  simpleEndOfDayPower: 690,
};

const orderError: SolarControlValidationError = {
  field: "socHigh",
  error: "must be greater than socLow",
  code: "order",
};

const setup = () => mount(ParamsCard, { props: { config, strategy: "smart" } });

const edit = async (wrapper: ReturnType<typeof setup>, key: string, value: string) => {
  const input = wrapper.get<HTMLInputElement>(`#solarControl_${key}`);
  input.element.value = value;
  await input.trigger("input");
  await input.trigger("change");
  return input;
};

describe("ParamsCard", () => {
  test("cross-field error on edited and reported field", async () => {
    const wrapper = setup();
    const socLow = await edit(wrapper, "socLow", "80");
    expect(wrapper.emitted("save")).toEqual([[{ socLow: 80 }]]);

    await wrapper.setProps({ errors: { socHigh: orderError, socLow: orderError } });
    expect(wrapper.text()).toContain("Must be lower than the high charge level.");
    expect(wrapper.text()).toContain("Must be greater than the low charge level.");
    expect(socLow.element.value).toBe("80");
    expect(wrapper.get<HTMLInputElement>("#solarControl_socHigh").element.value).toBe("75");
  });

  test("rejected value reverts when leaving the input", async () => {
    const wrapper = setup();
    await wrapper.setProps({ savingField: "socLow", savingState: "saving" });
    const socLow = await edit(wrapper, "socLow", "80");
    await socLow.trigger("blur");
    // still saving, keep the typed value
    expect(socLow.element.value).toBe("80");

    await wrapper.setProps({
      savingState: null,
      errors: { socHigh: orderError, socLow: orderError },
    });
    expect(socLow.element.value).toBe("80");
    await socLow.trigger("blur");
    expect(socLow.element.value).toBe("20");
  });

  test("rejected value reverts on the next successful save", async () => {
    const wrapper = setup();
    const socLow = await edit(wrapper, "socLow", "80");
    await wrapper.setProps({ errors: { socHigh: orderError, socLow: orderError } });

    await wrapper.setProps({ errors: {}, config: { ...config, minChargePower: 1500 } });
    expect(socLow.element.value).toBe("20");
  });

  test("range error from code", async () => {
    const wrapper = setup();
    await edit(wrapper, "minChargePower", "100");
    await wrapper.setProps({
      errors: {
        minChargePower: {
          field: "minChargePower",
          error: "must be between 500 and 11000",
          code: "range",
          min: 500,
          max: 11000,
        },
      },
    });
    expect(wrapper.text()).toContain("Allowed range: 500 to 11,000.");
  });

  test("raw error without code", async () => {
    const wrapper = setup();
    await wrapper.setProps({ errors: { socLow: { field: "", error: "bad json" } } });
    expect(wrapper.text()).toContain("bad json");
  });
});
