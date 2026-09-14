import { createStore } from "@tanstack/react-store";
import { ByteBuffer } from "flatbuffers";
import {
	candidatesAtom,
	observeSymbols,
	phaseAtom,
	positionCountAtom,
	updateEquity,
} from "./app";
import {
	LearningState,
	type LearningStateT,
} from "#/providers/telemetry/telemetry/learning-state";

// The decoded FlatBuffer is the single learning state consumed by every surface.
export const learningStore = createStore<LearningStateT | null>(null);

export const receiveLearning = (bytes: Uint8Array) => {
	const state = LearningState.getRootAsLearningState(
		new ByteBuffer(bytes),
	).unpack();

	learningStore.setState(() => state);

	if (state.markets && state.markets.length > 0) {
		const syms = state.markets
			.map((m) => String(m.symbol ?? ""))
			.filter(Boolean);
		if (syms.length > 0) {
			observeSymbols(syms);
		}
	}

	if (state.status) {
		phaseAtom.set(typeof state.status === "string" ? state.status : String(state.status));
	}

	if (state.decisions) {
		candidatesAtom.set(Number(state.decisions));
	}

	if (state.agents?.[0]) {
		const agent = state.agents[0];
		updateEquity(
			typeof agent.cash === "string" ? agent.cash : null,
			typeof agent.unrealized === "string" ? agent.unrealized : null,
			typeof agent.equity === "string" ? agent.equity : null,
		);
		positionCountAtom.set(
			agent.positions?.filter((p) => Number(p.holding?.qty) > 0).length ?? 0,
		);
	}
};

