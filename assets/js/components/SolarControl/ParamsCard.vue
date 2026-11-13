<template>
	<Card :title="$t('solarControl.params.title')" data-testid="solar-control-params">
		<section v-for="g in groups" :key="g.group" class="group" :aria-label="g.title">
			<CardHeader small class="mb-3" :title="g.title" :subtitle="g.subtitle" />
			<div class="row">
				<div v-for="p in g.params" :key="p.key" class="col-12 col-md-6">
					<FormRow
						:id="inputId(p.key)"
						:label="$t(`solarControl.params.${p.key}.label`)"
						:help="$t(`solarControl.params.${p.key}.help`)"
						:error="errorText(p.key)"
					>
						<div class="d-flex align-items-center flex-wrap column-gap-3 row-gap-1">
							<div class="input-group input-width">
								<input
									:id="inputId(p.key)"
									:value="config[p.key]"
									type="number"
									inputmode="decimal"
									:min="p.min"
									:max="p.max"
									:step="p.step"
									:aria-describedby="p.unit ? `${inputId(p.key)}Unit` : undefined"
									:aria-invalid="errors[p.key] ? 'true' : undefined"
									class="form-control text-end"
									:class="{ 'is-invalid': errors[p.key] }"
									@change="commit(p, $event)"
								/>
								<span
									v-if="p.unit"
									:id="`${inputId(p.key)}Unit`"
									class="input-group-text"
									>{{ unitLabel(p.unit) }}</span
								>
							</div>
							<span v-if="derived[p.key]" class="evcc-gray small">
								{{ derived[p.key] }}
							</span>
							<SavingIndicator v-if="savingField === p.key" :state="savingState" />
						</div>
					</FormRow>
				</div>
			</div>
		</section>
	</Card>
</template>

<script lang="ts">
import { defineComponent, type PropType } from "vue";
import formatter, { POWER_UNIT } from "@/mixins/formatter";
import Card from "../Helper/Card.vue";
import CardHeader from "../Helper/CardHeader.vue";
import FormRow from "../Config/FormRow.vue";
import SavingIndicator, { type SavingState } from "../Helper/SavingIndicator.vue";
import {
	PARAMS,
	paramDef,
	parseParam,
	shareOfMinPower,
	smartThresholds,
	type ParamDef,
	type ParamGroup,
	type ParamUnit,
} from "./params";
import type { SolarControlConfig, SolarControlConfigKey } from "./types";

interface Group {
	group: ParamGroup;
	title: string;
	subtitle: string;
	params: ParamDef[];
}

const GROUPS: ParamGroup[] = ["general", "smart", "simple"];

// cross-field rules of Config.Validate, reported on the second field
const CROSS_FIELD: Partial<Record<SolarControlConfigKey, string>> = {
	socHigh: "solarControl.params.socHigh.errorOrder",
	startHourTo: "solarControl.params.startHourTo.errorOrder",
};

export default defineComponent({
	name: "SolarControlParamsCard",
	components: { Card, CardHeader, FormRow, SavingIndicator },
	mixins: [formatter],
	props: {
		config: { type: Object as PropType<SolarControlConfig>, required: true },
		soc: { type: Number as PropType<number | null>, default: null },
		errors: { type: Object as PropType<Record<string, string>>, default: () => ({}) },
		savingField: { type: String as PropType<string | null>, default: null },
		savingState: { type: String as PropType<SavingState>, default: null },
	},
	emits: ["save"],
	watch: {
		errors(next: Record<string, string>, prev: Record<string, string>) {
			// a rejected value stays in its input until the error clears, then show the saved one
			Object.keys(prev)
				.filter((key) => !next[key] && paramDef(key))
				.forEach((key) => {
					const input = document.getElementById(this.inputId(key));
					if (input instanceof HTMLInputElement) {
						input.value = String(this.config[key as SolarControlConfigKey]);
					}
				});
		},
	},
	computed: {
		groups(): Group[] {
			return GROUPS.map((group) => ({
				group,
				title: this.$t(`solarControl.params.group.${group}`),
				subtitle:
					group === this.config.strategy ? this.$t("solarControl.params.inUse") : "",
				params: PARAMS.filter((p) => p.group === group),
			}));
		},
		derived(): Partial<Record<SolarControlConfigKey, string>> {
			const cfg = this.config;
			const equals = (w: number) =>
				this.$t("solarControl.params.equals", { value: this.fmtPower(w) });
			// soc dependent values show the range between both ends when the charge level is unknown
			const atSoc = (fn: (soc: number) => number) => {
				if (this.soc !== null) return equals(fn(this.soc));
				const low = fn(cfg.socLow);
				const high = fn(cfg.socHigh);
				return this.$t("solarControl.params.equalsRange", {
					min: this.fmtPower(Math.min(low, high)),
					max: this.fmtPower(Math.max(low, high)),
				});
			};
			return {
				startShareLowSoc: equals(shareOfMinPower(cfg, cfg.startShareLowSoc)),
				startShareHighSoc: equals(shareOfMinPower(cfg, cfg.startShareHighSoc)),
				stopHysteresis: atSoc((soc) => smartThresholds(cfg, soc).stop),
				baseHomeLoad: atSoc((soc) => smartThresholds(cfg, soc).pvStop),
				simpleStartShare: equals(shareOfMinPower(cfg, cfg.simpleStartShare)),
				simpleForecastShare: equals(shareOfMinPower(cfg, cfg.simpleForecastShare)),
			};
		},
	},
	methods: {
		inputId(key: string): string {
			return `solarControl_${key}`;
		},
		fmtPower(w: number): string {
			return this.fmtW(w, POWER_UNIT.W);
		},
		unitLabel(unit: ParamUnit): string {
			return unit === "min" ? this.$t("solarControl.params.unitMinutes") : unit;
		},
		errorText(key: SolarControlConfigKey): string | undefined {
			const raw = this.errors[key];
			if (!raw) return undefined;
			const cross = CROSS_FIELD[key];
			if (cross && raw.includes("greater than")) return this.$t(cross);
			const def = paramDef(key);
			if (def && raw.includes("between")) {
				return this.$t("solarControl.params.errorRange", {
					min: this.fmtNumber(def.min),
					max: this.fmtNumber(def.max),
				});
			}
			return raw;
		},
		commit(p: ParamDef, event: Event) {
			const input = event.target as HTMLInputElement;
			const value = parseParam(p, input.value);
			if (value === null) {
				input.value = String(this.config[p.key]);
				return;
			}
			input.value = String(value);
			if (value === this.config[p.key] && !this.errors[p.key]) return;
			this.$emit("save", { [p.key]: value });
		},
	},
});
</script>

<style scoped>
.group + .group {
	margin-top: 1rem;
	padding-top: 1.5rem;
	border-top: 1px solid var(--bs-border-color-translucent);
}
.input-width {
	width: 10rem;
}
</style>
