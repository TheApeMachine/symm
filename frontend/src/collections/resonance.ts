import { createStore } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
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
const CROSS_SECTION_HISTORY_LIMIT = 1;

const resonanceBuffer = (
	current: CircularBuffer<ResonanceFrame> | undefined,
	symbol: string,
): CircularBuffer<ResonanceFrame> => {
	const capacity =
		symbol === appStore.state.focusSymbol
			? RESONANCE_HISTORY_LIMIT
			: CROSS_SECTION_HISTORY_LIMIT;

	if (current?.capacity() === capacity) {
		return current;
	}

	const buffer = Circular<ResonanceFrame>(capacity);

	for (const frame of current?.values().slice(-capacity) ?? []) {
		buffer.push(frame);
	}

	return buffer;
};

/*
resonanceStore retains chart history for the focused symbol and only the latest
cross-section frame elsewhere so predictive history remains stable without
retaining every symbol's latent layers.
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
					resonance[frame.symbol] = resonanceBuffer(
						resonance[frame.symbol],
						frame.symbol,
					);

					resonance[frame.symbol].push(frame);
				}

				return {
					resonance,
					version: prev.version + 1,
				};
			}),
	}),
);
