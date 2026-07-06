import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

export type ResonanceFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
};

export const resonanceStore = createStore(
	{
		resonance: {} as Record<string, CircularBuffer<ResonanceFrame>>,
	},
	({ setState }) => ({
		updateFrame: (frames: ResonanceFrame[]) =>
			setState((prev) => {
				const resonance = { ...prev.resonance };

				for (const frame of frames) {
					if (!resonance[frame.symbol]) {
						resonance[frame.symbol] = Circular<ResonanceFrame>(50);
					}

					resonance[frame.symbol].push(frame);
				}

				return {
					resonance,
				};
			}),
	}),
);
