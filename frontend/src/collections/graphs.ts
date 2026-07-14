import { createStore } from "@tanstack/react-store";
import { mergeGraphFrames } from "#/collections/snapshot-retain";
import type { GraphFrame } from "#/types/thesis";

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
graphsStore retains the latest backend thesis evidence graph per symbol so open
positions can render the full measurement relationship topology on demand.
*/
export const graphsStore = createStore(
	{
		graphs: {} as Record<string, GraphFrame>,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => ({
				graphs: mergeGraphFrames(asGraphs(frame), prev.graphs),
			})),
		reset: () =>
			setState(() => ({
				graphs: {},
			})),
	}),
);
