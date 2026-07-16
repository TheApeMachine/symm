import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

/*
CausalReading mirrors algorithm.PearlOutput published inside each causal frame.
*/
export type CausalReading = {
	value?: number;
	category?: number;
	confidence?: number;
	confidenceBaseline?: number;
	entryBaseline?: number;
	exitBaseline?: number;
	strength?: number;
	association?: number;
	associationScore?: number;
	intervention?: number;
	interventionScore?: number;
	doExpectation?: number;
	uplift?: number;
	upliftScore?: number;
	counterfactual?: number;
	noise?: number;
	contagion?: number;
	condition?: number;
	inverted?: boolean;
	probabilities?: number[];
	distribution?: Record<string, number>;
};

/*
CausalFrame mirrors logic.CausalOutcome published on each thesis tick.
*/
export type CausalFrame = {
	source: string;
	symbol: string;
	at: string;
	samples?: number;
	ready?: boolean;
	hypothesis?: string;
	treatment?: string;
	controls?: string[];
	target?: string;
	reading?: CausalReading;
};

const CAUSAL_HISTORY_LIMIT = 1;

/*
causalStore retains the latest backend causal frame per symbol. Every consumer
reads the current causal outcome, so keeping superseded frames only increases
the cross-section memory footprint.
*/
export const causalStore = createStore(
	{
		causal: {} as Record<string, CircularBuffer<CausalFrame>>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frame: CausalFrame | CausalFrame[]) =>
			setState((prev) => {
				const frames = Array.isArray(frame) ? frame : [frame];

				if (frames.length === 0) {
					return prev;
				}

				const causal = prev.causal;

				for (const causalFrame of frames) {
					if (!causal[causalFrame.symbol]) {
						causal[causalFrame.symbol] =
							Circular<CausalFrame>(CAUSAL_HISTORY_LIMIT);
					}

					causal[causalFrame.symbol].push(causalFrame);
				}

				return {
					causal,
					version: prev.version + 1,
				};
			}),
	}),
);
