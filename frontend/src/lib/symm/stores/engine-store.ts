import { createStore } from "@tanstack/react-store";

import type { DecisionTraceEvent, EnginePulseEvent } from "#/lib/symm/events";
import {
	mergeSignalConfidences,
	type SignalConfidenceSnapshot,
} from "#/lib/symm/signal-confidence";

const MAX_PULSE_LOG = 32;

const emptySignalConfidences = (): SignalConfidenceSnapshot => ({
	hawkes: 0,
	fluid: 0,
	pumpdump: 0,
	causal: 0,
});

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
		signalConfidences: mergeSignalConfidences(state.signalConfidences, pulse),
	}));
};

export const applyDecisionTrace = (trace: DecisionTraceEvent): void => {
	engineStore.setState((state) => ({
		...state,
		decisionTrace: trace,
	}));
};
