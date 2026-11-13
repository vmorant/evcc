<template>
	<Card :title="$t('solarControl.status.title')" data-testid="solar-control-status">
		<template #actions>
			<SavingIndicator :state="savingState" />
		</template>

		<div class="d-flex flex-column flex-sm-row flex-wrap gap-3 mb-3">
			<div>
				<div class="evcc-gray small mb-1">{{ $t("solarControl.status.mode") }}</div>
				<SelectGroup
					id="solarControlMode"
					:model-value="mode"
					:options="modeOptions"
					:aria-label="$t('solarControl.status.mode')"
					equal-width
					class="selector"
					@update:model-value="changeMode"
				/>
				<div v-if="errors['mode']" class="invalid-feedback d-block">
					{{ errors["mode"] }}
				</div>
			</div>
			<div>
				<div class="evcc-gray small mb-1">{{ $t("solarControl.status.strategy") }}</div>
				<SelectGroup
					id="solarControlStrategy"
					:model-value="strategy"
					:options="strategyOptions"
					:aria-label="$t('solarControl.status.strategy')"
					equal-width
					class="selector"
					@update:model-value="changeStrategy"
				/>
				<div v-if="errors['strategy']" class="invalid-feedback d-block">
					{{ errors["strategy"] }}
				</div>
			</div>
		</div>

		<div v-if="isShadow" class="alert alert-warning py-2" role="status">
			{{ $t("solarControl.status.shadowWarning") }}
		</div>

		<template v-if="status">
			<p class="fs-5 mb-4" data-testid="solar-control-reason">{{ reasonText }}</p>

			<div class="row g-4 mb-4">
				<div v-for="c in comparisons" :key="c.key" class="col-12 col-md-4">
					<ThresholdBar
						:label="c.label"
						:value="c.value"
						:threshold="c.threshold"
						:valid="c.valid"
						:format="c.format"
					/>
				</div>
			</div>

			<div class="facts row g-3">
				<div v-for="f in facts" :key="f.key" class="col-6 col-md-3">
					<div class="evcc-gray small">{{ f.label }}</div>
					<div>{{ f.value }}</div>
				</div>
			</div>
		</template>
		<p v-else class="evcc-gray mb-0">{{ $t("solarControl.status.noData") }}</p>

		<div class="d-flex justify-content-end mt-3">
			<button
				type="button"
				class="btn btn-sm btn-outline-secondary"
				:disabled="!status"
				@click="$emit('reset')"
			>
				{{ $t("solarControl.status.reset") }}
			</button>
		</div>
	</Card>
</template>

<script lang="ts">
import { defineComponent, type PropType } from "vue";
import formatter, { POWER_UNIT } from "@/mixins/formatter";
import Card from "../Helper/Card.vue";
import SelectGroup from "../Helper/SelectGroup.vue";
import SavingIndicator, { type SavingState } from "../Helper/SavingIndicator.vue";
import ThresholdBar from "./ThresholdBar.vue";
import type {
	SolarControlConfig,
	SolarControlMode,
	SolarControlReason,
	SolarControlStatus,
	SolarControlStrategy,
} from "./types";
import type { SelectOption } from "@/types/evcc";

interface Comparison {
	key: string;
	label: string;
	value: number | null;
	threshold: number | null;
	valid: boolean;
	format: (n: number) => string;
}

interface Fact {
	key: string;
	label: string;
	value: string;
}

const MODES: SolarControlMode[] = ["off", "shadow", "active"];
const STRATEGIES: SolarControlStrategy[] = ["smart", "simple"];

export default defineComponent({
	name: "SolarControlStatusCard",
	components: { Card, SelectGroup, SavingIndicator, ThresholdBar },
	mixins: [formatter],
	props: {
		config: { type: Object as PropType<SolarControlConfig>, required: true },
		status: { type: Object as PropType<SolarControlStatus | null>, default: null },
		savingState: { type: String as PropType<SavingState>, default: null },
		errors: { type: Object as PropType<Record<string, string>>, default: () => ({}) },
	},
	emits: ["change-mode", "change-strategy", "reset"],
	computed: {
		mode(): SolarControlMode {
			return this.config.mode;
		},
		strategy(): SolarControlStrategy {
			return this.config.strategy;
		},
		isShadow(): boolean {
			return this.mode === "shadow";
		},
		isSimple(): boolean {
			return this.strategy === "simple";
		},
		modeOptions(): SelectOption<string>[] {
			return MODES.map((value) => ({ value, name: this.$t(`solarControl.mode.${value}`) }));
		},
		strategyOptions(): SelectOption<string>[] {
			return STRATEGIES.map((value) => ({
				value,
				name: this.$t(`solarControl.strategy.${value}`),
			}));
		},
		reason(): SolarControlReason | null {
			return this.status?.reason ?? null;
		},
		reasonKey(): string {
			const r = this.reason;
			if (!r) return "";
			switch (r.code) {
				case "charging":
					if (r.remaining != null) return "chargingStopping";
					return r.value != null && r.threshold != null ? "chargingValues" : "charging";
				case "ready":
					return r.remaining != null ? "readyIn" : "ready";
				case "surplusLow":
				case "forecastLow":
					return this.isSimple ? `${r.code}Simple` : r.code;
				default:
					return r.code;
			}
		},
		reasonText(): string {
			const r = this.reason;
			if (!r) return "";
			const percent = this.isSimple && r.code === "forecastLow";
			const fmt = (n?: number) => {
				if (n == null) return "–";
				return percent ? this.fmtPercentage(n) : this.fmtPower(n);
			};
			return this.$t(`solarControl.reason.${this.reasonKey}`, {
				value: fmt(r.value),
				threshold: fmt(r.threshold),
				remaining: this.fmtSeconds(r.remaining),
				count: r.count ?? "–",
				max: r.max ?? "–",
				from: this.fmtHour(r.from),
				to: this.fmtHour(r.to),
			});
		},
		comparisons(): Comparison[] {
			const s = this.status;
			if (!s) return [];
			const { values: v, valid } = s;
			const power = this.fmtPower;
			const percent = (n: number) => this.fmtPercentage(n);
			if (this.isSimple) {
				return [
					{
						key: "pvStart",
						label: this.$t("solarControl.status.pvAvg30Start"),
						value: v.pvAvg30,
						threshold: v.startThreshold,
						valid: valid.pvAvg30,
						format: power,
					},
					{
						key: "coverage",
						label: this.$t("solarControl.status.forecastCoverage"),
						value: v.forecastCoverage,
						threshold: this.config.simpleForecastShare,
						valid: true,
						format: percent,
					},
					{
						key: "pvStop",
						label: this.$t("solarControl.status.pvAvg30Stop"),
						value: v.pvAvg30,
						threshold: this.config.simpleStopPower,
						valid: valid.pvAvg30,
						format: power,
					},
				];
			}
			return [
				{
					key: "surplus",
					label: this.$t("solarControl.status.surplusAvg15"),
					value: v.surplusAvg15,
					threshold: v.startThreshold,
					valid: valid.surplusAvg15,
					format: power,
				},
				{
					key: "forecast",
					label: this.$t("solarControl.status.forecastSurplus2h"),
					value: v.forecastSurplus2h,
					threshold: v.startThreshold,
					valid: true,
					format: power,
				},
				{
					key: "pvStop",
					label: this.$t("solarControl.status.pvAvg15Stop"),
					value: v.pvAvg15,
					threshold: v.pvStopThreshold,
					valid: valid.pvAvg15,
					format: power,
				},
			];
		},
		facts(): Fact[] {
			const s = this.status;
			if (!s) return [];
			const facts: Fact[] = [
				{
					key: "starts",
					label: this.$t("solarControl.status.startsToday"),
					value: this.isSimple
						? `${s.startsToday}`
						: this.$t("solarControl.status.countOf", {
								count: s.startsToday,
								max: s.maxStartsPerDay,
							}),
				},
				{
					key: "cooldown",
					label: this.$t("solarControl.status.cooldown"),
					value: this.fmtSeconds(s.cooldownRemaining),
				},
				{
					key: "startHold",
					label: this.$t("solarControl.status.startHold"),
					value: this.fmtSeconds(s.startHoldRemaining),
				},
				{
					key: "stopHold",
					label: this.$t("solarControl.status.stopHold"),
					value: this.fmtSeconds(s.stopHoldRemaining),
				},
				{
					key: "soc",
					label: this.$t("solarControl.status.soc"),
					value: s.values.soc == null ? "–" : this.fmtPercentage(s.values.soc),
				},
				{
					key: "lastStart",
					label: this.$t("solarControl.status.lastStart"),
					value: this.fmtTime(s.lastStartAt),
				},
				{
					key: "lastStop",
					label: this.$t("solarControl.status.lastStop"),
					value: this.fmtTime(s.lastStopAt),
				},
			];
			if (!this.isSimple) {
				facts.splice(4, 0, {
					key: "sun",
					label: this.$t("solarControl.status.sun"),
					value: s.sunOk
						? this.$t("solarControl.status.sunOk")
						: this.$t("solarControl.status.sunLow"),
				});
			}
			return facts;
		},
	},
	methods: {
		fmtPower(n: number): string {
			return this.fmtW(n, POWER_UNIT.W);
		},
		fmtSeconds(seconds?: number | null): string {
			if (seconds == null || seconds <= 0) return "–";
			return this.fmtDuration(seconds, true, "m");
		},
		fmtHour(hour?: number): string {
			if (hour == null) return "–";
			const d = new Date();
			d.setHours(hour, 0, 0, 0);
			return this.fmtHourMinute(d);
		},
		fmtTime(time: string | null): string {
			if (!time) return "–";
			return this.fmtDayTime(new Date(time));
		},
		changeMode(value: SolarControlMode) {
			if (value !== this.mode) this.$emit("change-mode", value);
		},
		changeStrategy(value: SolarControlStrategy) {
			if (value !== this.strategy) this.$emit("change-strategy", value);
		},
	},
});
</script>

<style scoped>
@import "../../../css/breakpoints.css";
.selector {
	width: 100%;
}
@media (--sm-and-up) {
	.selector {
		width: 22rem;
	}
}
</style>
