<template>
	<div class="threshold-bar" role="group" :aria-label="label">
		<div class="d-flex justify-content-between align-items-baseline gap-2 flex-wrap">
			<span class="label">{{ label }}</span>
			<span class="text-nowrap">
				<strong>{{ valueText }}</strong>
				<span class="evcc-gray">&nbsp;/&nbsp;{{ thresholdText }}</span>
			</span>
		</div>
		<div class="track mt-1" aria-hidden="true">
			<div v-if="showFill" class="fill" :class="{ met }" :style="fillStyle"></div>
			<div v-if="threshold !== null" class="marker" :style="markerStyle"></div>
		</div>
		<small class="d-block evcc-gray mt-1">{{ stateText }}</small>
	</div>
</template>

<script lang="ts">
import { defineComponent, type PropType } from "vue";

// value vs threshold with a bar; the condition holds when the value is above the threshold
export default defineComponent({
	name: "SolarControlThresholdBar",
	props: {
		label: { type: String, required: true },
		value: { type: Number as PropType<number | null>, default: null },
		threshold: { type: Number as PropType<number | null>, default: null },
		valid: { type: Boolean, default: true },
		format: { type: Function as PropType<(n: number) => string>, required: true },
	},
	computed: {
		known(): boolean {
			return this.value !== null && this.threshold !== null;
		},
		showFill(): boolean {
			return this.value !== null && this.valid;
		},
		met(): boolean {
			return this.known && this.valid && (this.value as number) > (this.threshold as number);
		},
		scale(): number {
			const t = this.threshold ?? 0;
			const v = this.value ?? 0;
			return Math.max(t * 1.5, v, 1);
		},
		fillStyle(): Record<string, string> {
			const v = Math.max(this.value ?? 0, 0);
			return { width: `${Math.min((v / this.scale) * 100, 100)}%` };
		},
		markerStyle(): Record<string, string> {
			const t = Math.max(this.threshold ?? 0, 0);
			return { left: `${Math.min((t / this.scale) * 100, 100)}%` };
		},
		valueText(): string {
			return this.value === null ? "–" : this.format(this.value);
		},
		thresholdText(): string {
			return this.threshold === null ? "–" : this.format(this.threshold);
		},
		stateText(): string {
			if (this.value === null || this.threshold === null) {
				return this.$t("solarControl.status.unknown");
			}
			if (!this.valid) {
				return this.$t("solarControl.status.notValid");
			}
			return this.met
				? this.$t("solarControl.status.above")
				: this.$t("solarControl.status.below");
		},
	},
});
</script>

<style scoped>
.label {
	color: var(--evcc-gray);
}
.track {
	position: relative;
	height: 0.5rem;
	border-radius: 0.25rem;
	background-color: var(--evcc-box-border);
}
:root.dark .track {
	background-color: var(--evcc-background);
}
.fill {
	height: 100%;
	border-radius: 0.25rem;
	background-color: var(--evcc-gray);
	transition: width var(--evcc-transition-medium);
}
.fill.met {
	background-color: var(--evcc-darker-green);
}
.marker {
	position: absolute;
	top: -0.25rem;
	bottom: -0.25rem;
	width: 2px;
	margin-left: -1px;
	background-color: var(--evcc-default-text);
	transition: left var(--evcc-transition-medium);
}
</style>
