import { batch } from "@tanstack/store";
import { appStore } from "#/collections/app";
import { balancesStore } from "#/collections/balances";
import { categoriesStore } from "#/collections/categories";
import { causalStore } from "#/collections/causal";
import { cognitiveStore } from "#/collections/cognitive";
import { decisionStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import { findingsStore } from "#/collections/findings";
import { forecastsStore } from "#/collections/forecasts";
import { graphsStore } from "#/collections/graphs";
import { hypothesesStore } from "#/collections/hypotheses";
import { instrumentsStore } from "#/collections/instruments";
import { lifecycleStore } from "#/collections/lifecycle";
import { manifoldStore } from "#/collections/manifold";
import { expandWireMeasurements } from "#/collections/measurement-wire";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import { stopsStore } from "#/collections/stops";
import { terminalStore } from "#/collections/terminal";
import { tickStore } from "#/collections/tick";
import { tradeJournalStore } from "#/collections/trade-journal";
import { liveFocusSymbol } from "#/components/terminal/measurement-sources";

type FrameStore = {
	actions: {
		updateFrame: (frame: unknown) => void;
	};
};

export const frameStores = {
	balances: balancesStore,
	categories: categoriesStore,
	causal: causalStore,
	cognitive: cognitiveStore,
	decisions: decisionStore,
	diagnostics: diagnosticsStore,
	findings: findingsStore,
	forecasts: forecastsStore,
	graphs: graphsStore,
	hypotheses: hypothesesStore,
	lifecycle: lifecycleStore,
	tradeJournal: tradeJournalStore,
	positions: positionsStore,
	stops: stopsStore,
	executions: executionsStore,
	instruments: instrumentsStore,
	measurements: measurementsStore,
	manifold: manifoldStore,
	orders: ordersStore,
	resonance: resonanceStore,
	tick: tickStore,
} as Record<string, FrameStore>;

/*
applyFramePayload routes one coalesced worker payload into the TanStack stores
that already own each backend frame key. Tick updates first so the counter never
waits behind a large measurements ingest. Nested wire metrics maps expand into
flat readings before the measurements store sees them.
*/
export const applyFramePayload = (parsedData: Record<string, unknown>) => {
	batch(() => {
		if (parsedData.error !== undefined) {
			const frame = parsedData.error;

			if (
				frame !== null &&
				typeof frame === "object" &&
				!Array.isArray(frame)
			) {
				appStore.actions.updateError(frame as Record<string, unknown>);
			}
		}

		if (parsedData.tick !== undefined) {
			frameStores.tick.actions.updateFrame(parsedData.tick);
		}

		if (Array.isArray(parsedData.measurements)) {
			measurementsStore.actions.updateFrame(
				expandWireMeasurements(parsedData.measurements),
			);
			const preferred = appStore.state.focusSymbol;
			const live = liveFocusSymbol(measurementsStore.state, preferred);

			if (live !== preferred) {
				appStore.actions.updateFocusSymbol(live);
				terminalStore.actions.selectFocusSymbol(live);
			}
		}

		for (const [key, data] of Object.entries(parsedData)) {
			if (key === "measurements" || key === "tick" || key === "error") {
				continue;
			}

			const storeKey = key === "cognition" ? "cognitive" : key;

			if (frameStores[storeKey]?.actions) {
				frameStores[storeKey].actions.updateFrame(data);
				continue;
			}

			console.warn(`No store found matching frame key: "${key}"`);
		}
	});
};
