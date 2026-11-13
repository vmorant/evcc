<template>
	<section :aria-label="title" data-testid="solar-control-checks">
		<div class="evcc-gray small mb-1">{{ title }}</div>
		<ul class="list-unstyled mb-0">
			<li v-for="r in rows" :key="r.key" class="row-item d-flex gap-2">
				<span class="icon" :class="r.state" aria-hidden="true">{{ r.icon }}</span>
				<span>
					<span class="visually-hidden">{{ r.stateText }}: </span>{{ r.label }}
					<span v-if="r.detail" class="evcc-gray">· {{ r.detail }}</span>
				</span>
			</li>
		</ul>
	</section>
</template>

<script lang="ts">
import { defineComponent, type PropType } from "vue";
import formatter, { POWER_UNIT } from "@/mixins/formatter";
import { checkGap, checkState, checkUnit, formatHour, type CheckState } from "./decision";
import type {
	SolarControlCheck,
	SolarControlChecksPhase,
	SolarControlConfig,
	SolarControlStrategy,
} from "./types";

interface Row {
	key: string;
	state: CheckState;
	icon: string;
	label: string;
	detail: string;
	stateText: string;
}

const ICONS: Record<CheckState, string> = { ok: "✓", fail: "✗", unknown: "…" };

export default defineComponent({
	name: "SolarControlCheckList",
	mixins: [formatter],
	props: {
		checks: { type: Array as PropType<SolarControlCheck[]>, required: true },
		phase: { type: String as PropType<SolarControlChecksPhase>, required: true },
		strategy: { type: String as PropType<SolarControlStrategy>, required: true },
		config: { type: Object as PropType<SolarControlConfig>, required: true },
	},
	computed: {
		title(): string {
			return this.$t(`solarControl.check.title.${this.isStop ? "stop" : "start"}`);
		},
		isStop(): boolean {
			return this.phase === "stop";
		},
		rows(): Row[] {
			return this.checks.map((c) => {
				const state = checkState(c);
				return {
					key: c.key,
					state,
					icon: ICONS[state],
					label: this.label(c),
					detail: this.details(c, state).join(" · "),
					stateText: this.$t(`solarControl.check.state.${state}`),
				};
			});
		},
	},
	methods: {
		label(c: SolarControlCheck): string {
			if (this.isStop) return this.$t(`solarControl.check.stop.${c.key}`);
			if (c.key === "hold") {
				return this.$t("solarControl.check.start.hold", { min: this.config.startHoldMin });
			}
			if (c.key === "forecast" && this.strategy === "simple") {
				return this.$t("solarControl.check.start.forecastSimple");
			}
			return this.$t(`solarControl.check.start.${c.key}`);
		},
		fmt(n: number, c: SolarControlCheck, withUnit = true): string {
			if (checkUnit(c.key, this.strategy) === "%") {
				return withUnit ? this.fmtPercentage(n) : this.fmtNumber(n, 0);
			}
			return this.fmtW(n, POWER_UNIT.W, withUnit);
		},
		details(c: SolarControlCheck, state: CheckState): string[] {
			const result: string[] = [];
			const { value, threshold } = c;
			if (c.key === "hours" && value != null && threshold != null) {
				result.push(
					`${formatHour(value, this.fmtHourMinute)}–${formatHour(threshold, this.fmtHourMinute)}`
				);
			} else if (c.key === "dailyLimit" && value != null && threshold != null) {
				result.push(
					this.$t("solarControl.status.countOf", { count: value, max: threshold })
				);
			} else if (state === "unknown") {
				result.push(this.$t("solarControl.check.notValid"));
			} else if (
				checkUnit(c.key, this.strategy) !== "none" &&
				value != null &&
				threshold != null
			) {
				result.push(`${this.fmt(value, c, false)} / ${this.fmt(threshold, c)}`);
			}
			const gap = checkGap(c, this.phase);
			if (gap != null && checkUnit(c.key, this.strategy) !== "none") {
				result.push(this.$t("solarControl.check.missing", { value: this.fmt(gap, c) }));
			}
			if (c.remaining && c.remaining > 0) {
				const remaining = this.fmtDuration(c.remaining, true, "m");
				result.push(
					this.isStop
						? this.$t("solarControl.check.stopsIn", { remaining })
						: this.$t("solarControl.check.left", { remaining })
				);
			}
			return result;
		},
	},
});
</script>

<style scoped>
.row-item + .row-item {
	margin-top: 0.25rem;
}
.icon {
	flex: 0 0 1rem;
	text-align: center;
	font-weight: bold;
}
.icon.ok {
	color: var(--evcc-darker-green);
}
.icon.fail {
	color: var(--evcc-orange);
}
.icon.unknown {
	color: var(--evcc-gray);
}
</style>
