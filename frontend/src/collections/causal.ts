import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

export type CausalFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
};

export const causalStore = createStore(
	{
		causal: {} as Record<string, CircularBuffer<CausalFrame>>,
	},
	({ setState }) => ({
		updateFrame: (frames: CausalFrame[]) =>
			setState((prev) => {
				const causal = { ...prev.causal };

				for (const frame of frames) {
					if (!causal[frame.symbol]) {
						causal[frame.symbol] = Circular<CausalFrame>(50);
					}

					causal[frame.symbol].push(frame);
				}

				return {
					causal,
				};
			}),
	}),
);
