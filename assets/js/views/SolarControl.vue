<template>
	<div class="container px-4 safe-area-inset">
		<TopHeader :title="$t('solarControl.title')" />
		<div class="row">
			<main class="col-12">
				<template v-if="config">
					<StatusCard
						class="mb-4 box-pull-out"
						:config="config"
						:status="status"
						:saving-state="selectorSavingState"
						:errors="errors"
						@change-mode="changeMode"
						@change-strategy="changeStrategy"
						@reset="reset"
					/>
					<ParamsCard
						class="mb-4 box-pull-out"
						:config="config"
						:strategy="strategy"
						:soc="soc"
						:errors="errors"
						:saving-field="savingField"
						:saving-state="savingState"
						@save="save"
					/>
					<HistoryCard class="mb-4 box-pull-out" :events="events" />
				</template>
				<p v-else-if="loadError" class="my-4 text-muted">
					{{ $t("solarControl.unavailable") }}
				</p>
			</main>
		</div>
		<ConfirmModal
			ref="activateModal"
			id="solarControlActivateModal"
			:title="$t('solarControl.activate.title')"
			:description="$t('solarControl.activate.description')"
			:confirm-label="$t('solarControl.activate.confirm')"
			data-testid="solar-control-activate-modal"
		/>
	</div>
</template>

<script lang="ts">
import { defineComponent } from "vue";
import type { AxiosError } from "axios";
import store from "@/store";
import api from "@/api";
import auth from "../components/Auth/auth";
import type { State } from "@/types/evcc";
import Header from "../components/Top/Header.vue";
import ConfirmModal from "../components/Helper/ConfirmModal.vue";
import type { SavingState } from "../components/Helper/SavingIndicator.vue";
import StatusCard from "../components/SolarControl/StatusCard.vue";
import ParamsCard from "../components/SolarControl/ParamsCard.vue";
import HistoryCard from "../components/SolarControl/HistoryCard.vue";
import type {
	SolarControlConfig,
	SolarControlEvent,
	SolarControlMode,
	SolarControlResponse,
	SolarControlStatus,
	SolarControlStrategy,
	SolarControlValidationError,
} from "../components/SolarControl/types";

// solarControl is published by the solar control glue, not part of the global state type
type SolarControlState = State & { solarControl?: SolarControlStatus };

type ConfigUpdate = Partial<SolarControlConfig>;

export default defineComponent({
	name: "SolarControl",
	components: { TopHeader: Header, ConfirmModal, StatusCard, ParamsCard, HistoryCard },
	head() {
		return { title: this.$t("solarControl.title") };
	},
	data() {
		return {
			config: null as SolarControlConfig | null,
			fetchedStatus: null as SolarControlStatus | null,
			// a POST response is newer than the last websocket status until the next cycle
			preferFetched: false,
			loadError: false,
			// keyed by field, a cross-field error is shown on the reported and on the edited field
			errors: {} as Record<string, SolarControlValidationError>,
			savingField: null as string | null,
			savingState: null as SavingState,
			// update rejected for missing login, sent again after login
			pending: null as ConfigUpdate | "reset" | null,
		};
	},
	computed: {
		liveStatus(): SolarControlStatus | undefined {
			return (store.state as SolarControlState).solarControl;
		},
		status(): SolarControlStatus | null {
			if (this.preferFetched || !this.liveStatus) return this.fetchedStatus;
			return this.liveStatus;
		},
		strategy(): SolarControlStrategy {
			return this.status?.strategy ?? this.config?.strategy ?? "smart";
		},
		soc(): number | null {
			return this.status?.values.soc ?? null;
		},
		events(): SolarControlEvent[] {
			return this.status?.events ?? [];
		},
		loggedIn(): boolean | null {
			return auth.loggedIn;
		},
		selectorSavingState(): SavingState {
			return this.savingField === "mode" || this.savingField === "strategy"
				? this.savingState
				: null;
		},
	},
	watch: {
		liveStatus() {
			this.preferFetched = false;
		},
		loggedIn(loggedIn: boolean | null) {
			if (!loggedIn || !this.pending) return;
			const pending = this.pending;
			this.pending = null;
			if (pending === "reset") {
				this.reset();
			} else {
				this.save(pending);
			}
		},
	},
	mounted() {
		this.load();
	},
	methods: {
		async load() {
			try {
				const res = await api.get<SolarControlResponse>("solarcontrol");
				this.apply(res.data);
			} catch (e) {
				this.loadError = true;
				console.error(e);
			}
		},
		apply(data: SolarControlResponse) {
			this.config = data.config;
			this.fetchedStatus = data.status;
			this.preferFetched = true;
		},
		isUnauthorized(e: unknown): boolean {
			return (e as AxiosError)?.response?.status === 401;
		},
		async save(update: ConfigUpdate) {
			const field = Object.keys(update)[0] ?? null;
			this.savingField = field;
			this.savingState = "saving";
			try {
				const res = await api.post<SolarControlResponse | SolarControlValidationError>(
					"solarcontrol/config",
					update,
					{ validateStatus: (s) => (s >= 200 && s < 300) || s === 400 }
				);
				if (res.status === 400) {
					const err = res.data as SolarControlValidationError;
					const reported = err.field || field || "mode";
					this.errors = { [reported]: err };
					if (field) this.errors[field] = err;
					this.savingState = null;
					return;
				}
				this.errors = {};
				this.apply(res.data as SolarControlResponse);
				this.savingState = "saved";
			} catch (e) {
				this.savingState = null;
				// the api client opens the login dialog
				if (this.isUnauthorized(e)) this.pending = update;
				console.error(e);
			}
		},
		async changeMode(mode: SolarControlMode) {
			if (mode === "active") {
				const modal = this.$refs["activateModal"] as
					| InstanceType<typeof ConfirmModal>
					| undefined;
				if (!(await modal?.confirm())) return;
			}
			await this.save({ mode });
		},
		changeStrategy(strategy: SolarControlStrategy) {
			this.save({ strategy });
		},
		async reset() {
			try {
				const res = await api.post<SolarControlResponse | undefined>("solarcontrol/reset");
				if (res.data?.config) {
					this.apply(res.data);
				} else {
					await this.load();
				}
			} catch (e) {
				if (this.isUnauthorized(e)) this.pending = "reset";
				console.error(e);
			}
		},
	},
});
</script>
