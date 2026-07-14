import { createStore } from "@tanstack/react-store";
import type { GraphFrame, TradeObservation } from "#/types/thesis";

/*
mergeSnapshotArray merges thesis rows by identity and never drops retained rows
when a later websocket frame publishes an empty or partial snapshot.
*/
export const mergeSnapshotArray = <T>(
	incoming: T[],
	previous: T[],
	keyOf: (row: T) => string,
): T[] => {
	if (incoming.length === 0) {
		return previous;
	}

	const merged = new Map<string, T>();

	for (const row of previous) {
		merged.set(keyOf(row), row);
	}

	for (const row of incoming) {
		merged.set(keyOf(row), row);
	}

	return [...merged.values()];
};

/*
createSnapshotRowStore builds the shared snapshot-row collection used by thesis
category, forecast, and hypothesis stores so merge and reset behavior stay aligned.
*/
export const createSnapshotRowStore = <TRow, const TField extends string>(
	rowField: TField,
	asRows: (frame: unknown) => TRow[],
	rowKey: (row: TRow) => string,
) =>
	createStore({ [rowField]: [] } as Record<TField, TRow[]>, ({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => ({
				[rowField]: mergeSnapshotArray(asRows(frame), prev[rowField], rowKey),
			})),
		reset: () =>
			setState(() => ({
				[rowField]: [],
			})),
	}));

export const tradeObservationKey = (row: TradeObservation): string =>
	[
		row.kind,
		row.symbol,
		row.at,
		row.status ?? "",
		row.orderId ?? "",
		row.executionId ?? "",
		row.action ?? "",
	].join("\0");

/*
retainGraphFrame keeps the previous symbol graph when an incoming frame carries
no nodes or regresses to a sparser topology on a later thesis tick.
*/
export const retainGraphFrame = (
	incoming: GraphFrame,
	previous: GraphFrame | undefined,
): GraphFrame => {
	// ponytail: the node-count monotonicity check is an intentional heuristic that
	// may reject legitimate stale-relationship pruning; replace it with a stronger
	// freshness signal when one is available on the wire.
	if (previous === undefined) {
		return incoming;
	}

	if (incoming.nodes.length === 0) {
		return previous;
	}

	if (incoming.nodes.length < previous.nodes.length) {
		return previous;
	}

	return incoming;
};

/*
retainLifecycleMap merges only non-empty lifecycle states so blank wire values
cannot revert a symbol's retained lifecycle position.
*/
export const retainLifecycleMap = (
	incoming: Record<string, string>,
	previous: Record<string, string>,
): Record<string, string> => {
	const lifecycle = { ...previous };

	for (const [symbol, state] of Object.entries(incoming)) {
		if (typeof state !== "string" || state.length === 0) {
			continue;
		}

		lifecycle[symbol] = state;
	}

	return lifecycle;
};

export const forecastRowKey = (row: {
	symbol: string;
	source: string;
	target: string;
	sourceEpoch: number;
}): string => `${row.symbol}:${row.source}:${row.target}:${row.sourceEpoch}`;

export const hypothesisRowKey = (row: {
	symbol: string;
	source: string;
	claim: string;
}): string => `${row.symbol}:${row.source}:${row.claim}`;

export const categoryRowKey = (row: {
	symbol?: string;
	type: string;
}): string => `${row.symbol ?? ""}:${row.type}`;

/*
mergeGraphFrames merges symbol-local graphs without letting sparse frames erase
richer retained evidence for the same symbol.
*/
export const mergeGraphFrames = (
	incoming: GraphFrame[],
	previous: Record<string, GraphFrame>,
): Record<string, GraphFrame> => {
	if (incoming.length === 0) {
		return previous;
	}

	const graphs = { ...previous };

	for (const graph of incoming) {
		graphs[graph.symbol] = retainGraphFrame(graph, graphs[graph.symbol]);
	}

	return graphs;
};
