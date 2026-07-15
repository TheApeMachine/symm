import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

export type ManifoldFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
};

const asManifoldFrames = (
	frame: ManifoldFrame | ManifoldFrame[],
): ManifoldFrame[] => {
	const frames = Array.isArray(frame) ? frame : [frame];

	return frames.filter(
		(row): row is ManifoldFrame =>
			typeof row.symbol === "string" && row.symbol !== "",
	);
};

/*
manifoldStore retains backend manifold states per symbol in bounded circular
buffers so the fluid canvas can paint rho projections without React churn.
*/
export const manifoldStore = createStore(
	{
		manifold: {} as Record<string, CircularBuffer<ManifoldFrame>>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frame: ManifoldFrame | ManifoldFrame[]) =>
			setState((prev) => {
				const rows = asManifoldFrames(frame);

				if (rows.length === 0) {
					return prev;
				}

				const manifold = prev.manifold;

				for (const row of rows) {
					if (!manifold[row.symbol]) {
						manifold[row.symbol] = Circular<ManifoldFrame>(50);
					}

					manifold[row.symbol].push(row);
				}

				return {
					manifold,
					version: prev.version + 1,
				};
			}),
		reset: () =>
			setState(() => ({
				manifold: {},
				version: 0,
			})),
	}),
);
