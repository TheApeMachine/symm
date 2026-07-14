import { batch } from "@tanstack/store";
import { actionStore } from "#/collections/actions";
import { balancesStore } from "#/collections/balances";
import { causalStore } from "#/collections/causal";
import { cognitiveStore } from "#/collections/cognitive";
import { decisionStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import { findingsStore } from "#/collections/findings";
import { instrumentsStore } from "#/collections/instruments";
import { lifecycleStore } from "#/collections/lifecycle";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { ordersStore } from "#/collections/orders";
import { positionsStore } from "#/collections/positions";
import { resonanceStore } from "#/collections/resonance";
import { stopsStore } from "#/collections/stops";
import { tickStore } from "#/collections/tick";
import { tradeJournalStore } from "#/collections/trade-journal";
import type { Measurement } from "#/types/measurement";

type FrameStore = {
	actions: {
		updateFrame: (frame: unknown) => void;
	};
};

export const frameStores = {
	actions: actionStore,
	balances: balancesStore,
	causal: causalStore,
	cognitive: cognitiveStore,
	decisions: decisionStore,
	diagnostics: diagnosticsStore,
	findings: findingsStore,
	intents: actionStore,
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

const isMeasurementFrame = (value: unknown): value is Measurement[] =>
	Array.isArray(value);

/*
applyFramePayload routes one coalesced worker payload into the TanStack stores
that already own each backend frame key. Tick updates first so the counter never
waits behind a large measurements ingest.
*/
export const applyFramePayload = (parsedData: Record<string, unknown>) => {
	batch(() => {
		if (parsedData.tick !== undefined) {
			frameStores.tick.actions.updateFrame(parsedData.tick);
		}

		if (isMeasurementFrame(parsedData.measurements)) {
			measurementsStore.actions.updateFrame(parsedData.measurements);
		}

		for (const [key, data] of Object.entries(parsedData)) {
			if (key === "measurements" || key === "tick") {
				continue;
			}

			if (frameStores[key]?.actions) {
				frameStores[key].actions.updateFrame(data);
				continue;
			}

			console.warn(`No store found matching frame key: "${key}"`);
		}
	});
};
