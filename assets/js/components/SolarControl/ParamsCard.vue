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
									:value="inputValue(p.key)"
									type="number"
									inputmode="decimal"
									:min="p.min"
									:max="p.max"
									:step="p.step"
									:aria-describedby="p.unit ? `${inputId(p.key)}Unit` : undefined"
									:aria-invalid="errors[p.key] ? 'true' : undefined"
									class="form-control text-end"
									:class="{ 'is-invalid': errors[p.key] }"
									@input="edit(p, $event)"
									@change="commit(p, $event)"
									@blur="leave(p)"
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
	parseParam,
	shareOfMinPower,
	smartThresholds,
	type ParamDef,
	type ParamGroup,
	type ParamUnit,
} from "./params";
import type {
	SolarControlConfig,
	SolarControlConfigKey,
	SolarControlStrategy,
	SolarControlValidationError,
} from "./types";

interface Group {
	group: ParamGroup;
	title: string;
	subtitle: string;
	params: ParamDef[];
}

// input text that differs from the saved config while editing or after a rejected save
interface Draft {
	key: SolarControlConfigKey;
	value: string;
	saving: boolean;
}

const GROUPS: ParamGroup[] = ["general", "smart", "simple"];

// fields of the cross-field rules of Config.Validate, each with its own message
const ORDER_FIELDS: SolarControlConfigKey[] = ["socLow", "socHigh", "startHourFrom", "startHourTo"];

export default defineComponent({
	name: "SolarControlParamsCard",
	components: { Card, CardHeader, FormRow, SavingIndicator },
	mixins: [formatter],
	props: {
		config: { type: Object as PropType<SolarControlConfig>, required: true },
		strategy: { type: String as PropType<SolarControlStrategy>, required: true },
		soc: { type: Number as PropType<number | null>, default: null },
		errors: {
			type: Object as PropType<Record<string, SolarControlValidationError>>,
			default: () => ({}),
		},
		savingField: { type: String as PropType<string | null>, default: null },
		savingState: { type: String as PropType<SavingState>, default: null },
	},
	emits: ["save"],
	data() {
		return { draft: null as Draft | null };
	},
	watch: {
		config() {
			// saved: all inputs show the new config
			this.draft = null;
		},
		savingState(state: SavingState) {
			if (state !== "saving" && this.draft) this.draft.saving = false;
		},
	},
	computed: {
		groups(): Group[] {
			return GROUPS.map((group) => ({
				group,
				title: this.$t(`solarControl.params.group.${group}`),
				subtitle: group === this.strategy ? this.$t("solarControl.params.inUse") : "",
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
		inputValue(key: SolarControlConfigKey): string {
			return this.draft?.key === key ? this.draft.value : String(this.config[key]);
		},
		errorText(key: SolarControlConfigKey): string | undefined {
			const err = this.errors[key];
			if (!err) return undefined;
			if (err.code === "order" && ORDER_FIELDS.includes(key)) {
				return this.$t(`solarControl.params.${key}.errorOrder`);
			}
			if (err.code === "range" && err.min != null && err.max != null) {
				return this.$t("solarControl.params.errorRange", {
					min: this.fmtNumber(err.min),
					max: this.fmtNumber(err.max),
				});
			}
			return err.error;
		},
		edit(p: ParamDef, event: Event) {
			const { value } = event.target as HTMLInputElement;
			this.draft = { key: p.key, value, saving: false };
		},
		commit(p: ParamDef, event: Event) {
			const input = event.target as HTMLInputElement;
			const value = parseParam(p, input.value);
			if (value === null) {
				this.draft = null;
				input.value = String(this.config[p.key]);
				return;
			}
			if (value === this.config[p.key] && !this.errors[p.key]) {
				this.draft = null;
				input.value = String(value);
				return;
			}
			this.draft = { key: p.key, value: String(value), saving: true };
			input.value = this.draft.value;
			this.$emit("save", { [p.key]: value });
		},
		leave(p: ParamDef) {
			// a rejected value is shown until the input is left
			if (this.draft?.key === p.key && !this.draft.saving) this.draft = null;
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
