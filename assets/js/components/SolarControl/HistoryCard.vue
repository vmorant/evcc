<template>
	<Card :title="$t('solarControl.history.title')" data-testid="solar-control-history">
		<ul v-if="entries.length" class="list-unstyled mb-0">
			<li v-for="e in entries" :key="e.key" class="entry d-flex gap-3">
				<time class="time evcc-gray" :datetime="e.time">{{ e.timeText }}</time>
				<div class="flex-grow-1 min-w-0">
					<div>
						{{ e.title }}
						<small class="evcc-gray ms-1">{{ e.strategy }}</small>
					</div>
					<small v-if="e.details.length" class="d-block evcc-gray">
						<template v-for="(d, i) in e.details" :key="i"
							><template v-if="i > 0"> · </template>{{ d }}</template
						>
					</small>
				</div>
			</li>
		</ul>
		<p v-else class="evcc-gray mb-0">{{ $t("solarControl.history.empty") }}</p>
	</Card>
</template>

<script lang="ts">
import { defineComponent, type PropType } from "vue";
import formatter, { POWER_UNIT } from "@/mixins/formatter";
import Card from "../Helper/Card.vue";
import type { SolarControlEvent } from "./types";

interface Entry {
	key: string;
	time: string;
	timeText: string;
	title: string;
	strategy: string;
	details: string[];
}

const MAX_EVENTS = 30;

export default defineComponent({
	name: "SolarControlHistoryCard",
	components: { Card },
	mixins: [formatter],
	props: {
		// most recent first
		events: { type: Array as PropType<SolarControlEvent[]>, default: () => [] },
	},
	computed: {
		entries(): Entry[] {
			return this.events.slice(0, MAX_EVENTS).map((e, i) => ({
				key: `${e.time}-${i}`,
				time: e.time,
				timeText: this.fmtDayTime(new Date(e.time)),
				title: this.title(e),
				strategy: this.$t(`solarControl.strategy.${e.strategy}`),
				details: this.details(e),
			}));
		},
	},
	methods: {
		fmtPower(w: number): string {
			return this.fmtW(w, POWER_UNIT.W);
		},
		title(e: SolarControlEvent): string {
			if (e.type === "shadow" || e.type === "external") {
				return this.$t(`solarControl.history.${e.type}.${e.action ?? "start"}`);
			}
			return this.$t(`solarControl.history.type.${e.type}`);
		},
		details(e: SolarControlEvent): string[] {
			const result: string[] = [];
			if (e.rule) {
				result.push(this.$t(`solarControl.history.rule.${e.rule}`));
			}
			const threshold = e.threshold != null ? this.fmtPower(e.threshold) : "–";
			if (e.surplus != null) {
				result.push(
					this.$t("solarControl.history.surplus", {
						value: this.fmtPower(e.surplus),
						threshold,
					})
				);
			}
			if (e.pv != null) {
				result.push(
					this.$t("solarControl.history.pv", { value: this.fmtPower(e.pv), threshold })
				);
			}
			if (e.energy != null) {
				result.push(
					this.$t("solarControl.history.energy", { value: this.fmtKWh(e.energy) })
				);
			}
			if (e.solarPercentage != null) {
				result.push(
					this.$t("solarControl.history.solarPercentage", {
						value: this.fmtPercentage(e.solarPercentage),
					})
				);
			}
			if (e.resetMode) {
				result.push(this.$t("solarControl.history.resetMode"));
			}
			if (e.resetAlways) {
				result.push(this.$t("solarControl.history.resetAlways"));
			}
			return result;
		},
	},
});
</script>

<style scoped>
@import "../../../css/breakpoints.css";
.entry + .entry {
	margin-top: 0.75rem;
	padding-top: 0.75rem;
	border-top: 1px solid var(--bs-border-color-translucent);
}
.time {
	flex: 0 0 5.5rem;
}
@media (--sm-and-up) {
	.time {
		flex-basis: 9rem;
	}
}
.min-w-0 {
	min-width: 0;
}
</style>
