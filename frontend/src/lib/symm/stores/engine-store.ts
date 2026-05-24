import { createStore } from "@tanstack/react-store";

import type {
	DecisionTraceEvent,
	EnginePulseEvent,
	SignalScoreEvent,
} from "#/lib/symm/events";
import {
	emptySignalConfidences,
	isSignalSource,
	type SignalConfidenceSnapshot,
} from "#/lib/symm/signal-confidence";

const MAX_PULSE_LOG = 32;

export type EngineStoreState = {
	enginePulse?: EnginePulseEvent;
	decisionTrace?: DecisionTraceEvent;
	pulseLog: EnginePulseEvent[];
	signalConfidences: SignalConfidenceSnapshot;
};

export const engineStore = createStore<EngineStoreState>({
	pulseLog: [],
	signalConfidences: emptySignalConfidences(),
});

export const applyEnginePulse = (pulse: EnginePulseEvent): void => {
	engineStore.setState((state) => ({
		...state,
		enginePulse: pulse,
		pulseLog: [pulse, ...state.pulseLog].slice(0, MAX_PULSE_LOG),
	}));
};

export const applySignalScore = (event: SignalScoreEvent): void => {
	if (!isSignalSource(event.source)) {
		return;
	}

	const confidence =
		typeof event.confidence === "number" && Number.isFinite(event.confidence)
			? event.confidence
			: 0;

	engineStore.setState((state) => ({
		...state,
		signalConfidences: {
			...state.signalConfidences,
			[event.source]: confidence,
		},
	}));
};

export const applyDecisionTrace = (trace: DecisionTraceEvent): void => {
	engineStore.setState((state) => ({
		...state,
		decisionTrace: trace,
	}));
};
