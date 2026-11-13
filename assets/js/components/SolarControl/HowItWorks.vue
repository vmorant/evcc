<template>
	<details class="how" data-testid="solar-control-how">
		<summary class="evcc-gray">{{ $t("solarControl.howItWorks.title") }}</summary>
		<div class="mt-2 small">
			<p v-for="p in paragraphs" :key="p.key" class="mb-2">{{ p.text }}</p>
			<ul v-if="rules.length" class="mb-0 ps-3">
				<li v-for="r in rules" :key="r.key">{{ r.text }}</li>
			</ul>
		</div>
	</details>
</template>

<script lang="ts">
import { defineComponent, type PropType } from "vue";
import formatter, { POWER_UNIT } from "@/mixins/formatter";
import { formatHour } from "./decision";
import { shareOfMinPower, smartThresholds, type SmartThresholds } from "./params";
import type { SolarControlConfig, SolarControlStrategy } from "./types";

interface Text {
	key: string;
	text: string;
}

// explains the rules of a strategy with the current numbers
export default defineComponent({
	name: "SolarControlHowItWorks",
	mixins: [formatter],
	props: {
		strategy: { type: String as PropType<SolarControlStrategy>, required: true },
		config: { type: Object as PropType<SolarControlConfig>, required: true },
		soc: { type: Number as PropType<number | null>, default: null },
	},
	computed: {
		thresholds(): SmartThresholds | null {
			return this.soc === null ? null : smartThresholds(this.config, this.soc);
		},
		paragraphs(): Text[] {
			return this.strategy === "simple" ? this.simpleParagraphs : this.smartParagraphs;
		},
		smartParagraphs(): Text[] {
			const cfg = this.config;
			const t = this.thresholds;
			const start = t ? this.fmtPower(t.start) : "–";
			const values = {
				start,
				stop: t ? this.fmtPower(t.pvStop) : "–",
				hold: cfg.startHoldMin,
				stopHold: cfg.stopHoldMin,
				max: cfg.maxStartsPerDay,
				from: formatHour(cfg.startHourFrom, this.fmtHourMinute),
				to: formatHour(cfg.startHourTo, this.fmtHourMinute),
				soc: this.soc === null ? "–" : this.fmtPercentage(this.soc),
				hysteresis: this.fmtPercentage(cfg.stopHysteresis),
				cooldown: cfg.restartCooldownMin,
				minPower: this.fmtPower(cfg.minChargePower),
				share: t ? this.fmtPercentage((t.start / cfg.minChargePower) * 100) : "–",
			};
			return ["summary", "start", "forecast", "stop", "threshold", "now", "grid"].map(
				(key) => ({ key, text: this.$t(`solarControl.howItWorks.smart.${key}`, values) })
			);
		},
		simpleParagraphs(): Text[] {
			const cfg = this.config;
			const values = {
				start: this.fmtPower(shareOfMinPower(cfg, cfg.simpleStartShare)),
				coverage: this.fmtPercentage(cfg.simpleForecastShare),
			};
			return ["summary", "start", "stop"].map((key) => ({
				key,
				text: this.$t(`solarControl.howItWorks.simple.${key}`, values),
			}));
		},
		rules(): Text[] {
			if (this.strategy !== "simple") return [];
			const cfg = this.config;
			return [
				{ key: "power", value: this.fmtPower(cfg.simpleStopPower) },
				{ key: "share", value: this.fmtPercentage(cfg.simpleShareStop) },
				{ key: "endOfDay", value: this.fmtPower(cfg.simpleEndOfDayPower) },
			].map(({ key, value }) => ({
				key,
				text: this.$t(`solarControl.howItWorks.simple.${key}`, { value }),
			}));
		},
	},
	methods: {
		fmtPower(w: number): string {
			return this.fmtW(w, POWER_UNIT.W);
		},
	},
});
</script>

<style scoped>
.how summary {
	cursor: pointer;
}
</style>
