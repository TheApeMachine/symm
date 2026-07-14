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

const CAUSAL_HISTORY_LIMIT = 130;

/*
causalStore retains backend causal frames in bounded circular buffers so
websocket ingest and direct paint stay off the React render path.
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

				for (const frame of frames) {
					if (!causal[frame.symbol]) {
						causal[frame.symbol] = Circular<CausalFrame>(CAUSAL_HISTORY_LIMIT);
					}

					causal[frame.symbol].push(frame);
				}

				return {
					causal,
					version: prev.version + 1,
				};
			}),
	}),
);
