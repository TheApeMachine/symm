import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

export type ManifoldFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
};

export const manifoldStore = createStore(
	{
		manifold: {} as Record<string, CircularBuffer<ManifoldFrame>>,
	},
	({ setState }) => ({
		updateFrame: (frame: ManifoldFrame | ManifoldFrame[]) =>
			setState((prev) => {
				const manifold = { ...prev.manifold };
				const frames = Array.isArray(frame) ? frame : [frame];

				for (const frame of frames) {
					if (!manifold[frame.symbol]) {
						manifold[frame.symbol] = Circular<ManifoldFrame>(50);
					}

					manifold[frame.symbol].push(frame);
				}

				return {
					manifold,
				};
			}),
	}),
);
