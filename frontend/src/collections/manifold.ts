import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

export type ManifoldFrame = Record<string, unknown> & {
	source: string;
	symbol: string;
	at: string;
};

const MANIFOLD_HISTORY_LIMIT = 1;

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
manifoldStore retains the latest backend manifold state per symbol so the fluid
canvas can paint its dense projection without retaining superseded matrices.
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
						manifold[row.symbol] = Circular<ManifoldFrame>(
							MANIFOLD_HISTORY_LIMIT,
						);
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
