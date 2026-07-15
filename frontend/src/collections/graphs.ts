import { createStore } from "@tanstack/react-store";
import type { GraphFrame } from "#/types/thesis";
import { Circular, type CircularBuffer } from "./circular";

const GRAPH_HISTORY_LIMIT = 50;

const asGraphs = (frame: unknown): GraphFrame[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is GraphFrame =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as GraphFrame).symbol === "string" &&
			Array.isArray((row as GraphFrame).nodes) &&
			Array.isArray((row as GraphFrame).edges),
	);
};

/*
latestGraphFrame returns the newest retained evidence graph for one symbol.
*/
export const latestGraphFrame = (
	graphs: Record<string, CircularBuffer<GraphFrame>>,
	symbol: string,
): GraphFrame | null => graphs[symbol]?.values().at(-1) ?? null;

/*
graphsStore retains backend thesis evidence graphs per symbol in bounded
circular buffers so partial empty frames cannot erase populated topology.
*/
export const graphsStore = createStore(
	{
		graphs: {} as Record<string, CircularBuffer<GraphFrame>>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const rows = asGraphs(frame);

				if (rows.length === 0) {
					return prev;
				}

				const graphs = prev.graphs;

				for (const row of rows) {
					if (!graphs[row.symbol]) {
						graphs[row.symbol] = Circular<GraphFrame>(GRAPH_HISTORY_LIMIT);
					}

					const previous = graphs[row.symbol].values().at(-1);

					if (previous !== undefined && row.nodes.length === 0) {
						continue;
					}

					if (
						previous !== undefined &&
						row.nodes.length < previous.nodes.length
					) {
						continue;
					}

					graphs[row.symbol].push(row);
				}

				return {
					graphs,
					version: prev.version + 1,
				};
			}),
		reset: () =>
			setState(() => ({
				graphs: {},
				version: 0,
			})),
	}),
);
