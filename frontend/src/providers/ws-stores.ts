import { createKeyedStore } from "#/collections/store";
import type {
	Balance,
	CausalFrame,
	CognitiveReading,
	DiagnosticsFrame,
	Execution,
	Finding,
	GraphFrame,
	Holding,
	Instrument,
	LifecycleRow,
	ManifoldFrame,
	Order,
	ResonanceFrame,
	Stop,
	StrategyDecision,
	ThesisForecast,
	ThesisHypothesis,
	TickFrame,
} from "#/collections/types";
import { isPlainObject } from "#/providers/ws-frame-merge";
import type { Category, Measurement } from "#/types/measurement";
import type { Subscription } from "@tanstack/store";

const DEFAULT_BUFFER = 50;

export const frameStores = {
	balances: createKeyedStore<Balance>()("balances", 1, (row) => row.asset),
	categories: createKeyedStore<Category>()(
		"categories",
		DEFAULT_BUFFER,
		(row) => row.type,
	),
	causal: createKeyedStore<CausalFrame>()("causal", DEFAULT_BUFFER, (row) => row.symbol),
	cognitive: createKeyedStore<CognitiveReading>()("cognitive", DEFAULT_BUFFER, (row) => row.symbol),
	decisions: createKeyedStore<StrategyDecision>()("decisions", DEFAULT_BUFFER, (row) => row.symbol),
	diagnostics: createKeyedStore<DiagnosticsFrame>()("diagnostics", DEFAULT_BUFFER),
	findings: createKeyedStore<Finding>()("findings", DEFAULT_BUFFER, (row) => row.symbol),
	forecasts: createKeyedStore<ThesisForecast>()("forecasts", DEFAULT_BUFFER, (row) => row.symbol),
	graphs: createKeyedStore<GraphFrame>()("graphs", DEFAULT_BUFFER, (row) => row.symbol),
	hypotheses: createKeyedStore<ThesisHypothesis>()("hypotheses", DEFAULT_BUFFER, (row) => row.symbol),
	lifecycle: createKeyedStore<LifecycleRow>()("lifecycle", DEFAULT_BUFFER, (row) => row.symbol),
	holdings: createKeyedStore<Holding>()("holdings", DEFAULT_BUFFER, (row) => row.symbol),
	stops: createKeyedStore<Stop>()("stops", 1, (row) => row.symbol),
	executions: createKeyedStore<Execution>()("executions", DEFAULT_BUFFER),
	instruments: createKeyedStore<Instrument>()("instruments", DEFAULT_BUFFER, (row) => row.symbol),
	measurements: createKeyedStore<Measurement>()(
		"measurements",
		50,
		(row) => row.symbol,
		(row) => row.source,
	),
	manifold: createKeyedStore<ManifoldFrame>()("manifold", DEFAULT_BUFFER, (row) => row.symbol),
	orders: createKeyedStore<Order>()("orders", DEFAULT_BUFFER, (row) => row.pair),
	resonance: createKeyedStore<ResonanceFrame>()("resonance", DEFAULT_BUFFER, (row) => row.symbol),
	tick: createKeyedStore<TickFrame>()("tick", DEFAULT_BUFFER),
};

export const subscribe = <TState, T>(
	store: {
		subscribe: (listener: (state: TState) => void) => Subscription;
	},
	pick: (state: TState) => { values: () => T[] } | undefined,
	send: (rows: T[]) => void,
): Subscription =>
	store.subscribe((state) => {
		send(pick(state)?.values() ?? []);
	});

/*
applyFramePayload writes a coalesced websocket object into the keyed stores.
Error frames are returned so the worker can post them to the UI thread.
*/
export const applyFramePayload = (
	payload: Record<string, unknown>,
): Record<string, unknown> | null => {
	if (isPlainObject(payload.error)) {
		return payload.error;
	}

	for (const [name, value] of Object.entries(payload)) {
		// Backend publishCognition emits "cognition"; UI stores subscribe as "cognitive".
		const storeName = name === "cognition" ? "cognitive" : name;
		const store = frameStores[storeName as keyof typeof frameStores];

		if (store === undefined) {
			continue;
		}

		const rows = Array.isArray(value)
			? value
			: isPlainObject(value) &&
					Object.values(value).every((entry) => isPlainObject(entry))
				? Object.values(value)
				: value != null
					? [value]
					: [];

		if (rows.length === 0) {
			continue;
		}

		(store.actions as { updateFrame: (rows: unknown[]) => void }).updateFrame(
			rows,
		);
	}

	return null;
};