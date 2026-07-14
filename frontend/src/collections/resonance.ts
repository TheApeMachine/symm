import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

export type ResonanceLayer = {
	state: number[];
	prediction: number[];
};

/*
ResonanceFrame mirrors the backend ResonanceOutcome wire payload published on
each thesis tick. Extra wire keys remain accessible without forcing a second
store shape.
*/
export type ResonanceFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
	samples?: number;
	observables?: number[];
	latent?: number[];
	layers?: ResonanceLayer[];
	energy?: number;
	surprise?: number;
};

const RESONANCE_HISTORY_LIMIT = 130;

/*
resonanceStore retains backend resonance frames in bounded circular buffers so
websocket ingest and canvas paint stay off the React render path.
*/
export const resonanceStore = createStore(
	{
		resonance: {} as Record<string, CircularBuffer<ResonanceFrame>>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frame: ResonanceFrame | ResonanceFrame[]) =>
			setState((prev) => {
				const frames = Array.isArray(frame) ? frame : [frame];

				if (frames.length === 0) {
					return prev;
				}

				const resonance = prev.resonance;

				for (const frame of frames) {
					if (!resonance[frame.symbol]) {
						resonance[frame.symbol] = Circular<ResonanceFrame>(
							RESONANCE_HISTORY_LIMIT,
						);
					}

					resonance[frame.symbol].push(frame);
				}

				return {
					resonance,
					version: prev.version + 1,
				};
			}),
	}),
);
