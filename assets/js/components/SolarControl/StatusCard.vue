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
				<div v-if="modeError" class="invalid-feedback d-block">
					{{ modeError }}
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
				<div v-if="strategyError" class="invalid-feedback d-block">
					{{ strategyError }}
				</div>
			</div>
		</div>

		<div v-if="isShadow" class="alert alert-warning py-2" role="status">
			{{ $t("solarControl.status.shadowWarning") }}
		</div>

		<template v-if="status">
			<p class="fs-5 mb-1" data-testid="solar-control-reason">{{ reasonText }}</p>
			<p v-if="verdictText" class="mb-0" data-testid="solar-control-verdict">
				{{ verdictText }}
			</p>

			<CheckList
				v-if="status.checks.length"
				class="mt-3"
				:checks="status.checks"
				:phase="status.checksPhase"
				:strategy="strategy"
				:config="config"
			/>

			<div v-if="facts.length" class="row g-3 mt-1">
				<div v-for="f in facts" :key="f.key" class="col-6 col-md-3">
					<div class="evcc-gray small">{{ f.label }}</div>
					<div>{{ f.value }}</div>
				</div>
			</div>

			<HowItWorks
				class="mt-3"
				:strategy="strategy"
				:config="config"
				:soc="status.values.soc"
			/>
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
import CheckList from "./CheckList.vue";
import HowItWorks from "./HowItWorks.vue";
import { formatHour, verdict, type Verdict } from "./decision";
import type {
	SolarControlConfig,
	SolarControlMode,
	SolarControlReason,
	SolarControlStatus,
	SolarControlStrategy,
	SolarControlValidationError,
} from "./types";
import type { SelectOption } from "@/types/evcc";

interface Fact {
	key: string;
	label: string;
	value: string | null;
}

const MODES: SolarControlMode[] = ["off", "shadow", "active"];
const STRATEGIES: SolarControlStrategy[] = ["smart", "simple"];
// fixed duration of the simple stop rule on solar power
const SIMPLE_STOP_MIN = 10;

export default defineComponent({
	name: "SolarControlStatusCard",
	components: { Card, SelectGroup, SavingIndicator, CheckList, HowItWorks },
	mixins: [formatter],
	props: {
		config: { type: Object as PropType<SolarControlConfig>, required: true },
		status: { type: Object as PropType<SolarControlStatus | null>, default: null },
		savingState: { type: String as PropType<SavingState>, default: null },
		errors: {
			type: Object as PropType<Record<string, SolarControlValidationError>>,
			default: () => ({}),
		},
	},
	emits: ["change-mode", "change-strategy", "reset"],
	computed: {
		// live status follows changes made by other clients, the config only after reload
		mode(): SolarControlMode {
			return this.status?.mode ?? this.config.mode;
		},
		strategy(): SolarControlStrategy {
			return this.status?.strategy ?? this.config.strategy;
		},
		modeError(): string | undefined {
			return this.errors["mode"]?.error;
		},
		strategyError(): string | undefined {
			return this.errors["strategy"]?.error;
		},
		isShadow(): boolean {
			return this.mode === "shadow";
		},
		isSimple(): boolean {
			return this.strategy === "simple";
		},
		isCharging(): boolean {
			return this.status?.checksPhase === "stop";
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
					return r.remaining ? "readyIn" : "readyNow";
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
		verdict(): Verdict | null {
			const s = this.status;
			if (!s || this.mode === "off") return null;
			return verdict(s.checks, s.checksPhase, s.values, new Date());
		},
		verdictText(): string {
			const v = this.verdict;
			if (!v) return "";
			if (v.key === "charging") {
				return this.isSimple
					? this.$t("solarControl.verdict.chargingSimple", {
							stop: this.fmtPower(this.config.simpleStopPower),
							min: SIMPLE_STOP_MIN,
						})
					: this.$t("solarControl.verdict.chargingSmart", {
							stop: this.fmtNullablePower(this.status?.values.pvStopThreshold),
							min: this.config.stopHoldMin,
						});
			}
			return this.$t(`solarControl.verdict.${v.key}`, {
				remaining: this.fmtSeconds(v.remaining),
				time: v.time ? this.fmtHourMinute(v.time) : "–",
				from: this.fmtHour(v.from),
				to: this.fmtHour(v.to),
			});
		},
		facts(): Fact[] {
			const s = this.status;
			if (!s) return [];
			const v = s.values;
			const power = (n: number | null) => (n === null ? null : this.fmtPower(n));
			const time = (t: string | null) => (t ? this.fmtHourMinute(new Date(t)) : null);
			const facts: Fact[] = [
				{ key: "pv", label: this.$t("solarControl.status.pvNow"), value: power(v.pv) },
				{
					key: "home",
					label: this.$t("solarControl.status.homeNow"),
					value: power(v.home),
				},
			];
			if (this.isCharging) {
				facts.push(
					{
						key: "chargePower",
						label: this.$t("solarControl.status.chargePower"),
						value: power(v.chargePower),
					},
					{
						key: "session",
						label: this.$t("solarControl.status.session"),
						value: this.sessionText,
					}
				);
			} else {
				const windowStart = time(v.forecastWindowStart);
				const windowEnd = time(v.forecastWindowEnd);
				facts.push(
					{
						key: "surplus",
						label: this.$t("solarControl.status.surplusNow"),
						value: power(v.surplus),
					},
					{
						key: "forecast",
						label: this.$t("solarControl.status.forecastRestOfDay"),
						value:
							v.forecastEnergyRestOfDay === null
								? null
								: this.fmtWh(v.forecastEnergyRestOfDay, POWER_UNIT.AUTO),
					},
					{
						key: "sunEnd",
						label: this.$t("solarControl.status.sunEnd"),
						value: time(v.sunEnd),
					},
					{
						key: "window",
						label: this.$t("solarControl.status.forecastWindow"),
						value: windowStart && windowEnd ? `${windowStart}–${windowEnd}` : null,
					}
				);
			}
			if (!this.isSimple) {
				facts.push({
					key: "soc",
					label: this.$t("solarControl.status.soc"),
					value: v.soc === null ? null : this.fmtPercentage(v.soc),
				});
			}
			return facts.filter((f) => f.value !== null);
		},
		sessionText(): string | null {
			const v = this.status?.values;
			if (v?.sessionEnergy == null) return null;
			const energy = this.fmtKWh(v.sessionEnergy);
			if (v.sessionSolarPercentage == null) return energy;
			const solar = this.$t("solarControl.history.solarPercentage", {
				value: this.fmtPercentage(v.sessionSolarPercentage),
			});
			return `${energy} · ${solar}`;
		},
	},
	methods: {
		fmtPower(n: number): string {
			return this.fmtW(n, POWER_UNIT.W);
		},
		fmtNullablePower(n?: number | null): string {
			return n == null ? "–" : this.fmtPower(n);
		},
		fmtSeconds(seconds?: number | null): string {
			if (seconds == null || seconds <= 0) return "–";
			return this.fmtDuration(seconds, true, "m");
		},
		fmtHour(hour?: number): string {
			if (hour == null) return "–";
			return formatHour(hour, this.fmtHourMinute);
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
