import { createStore } from "@tanstack/react-store";
import type { StrategyDecision } from "#/types/thesis";
import { Circular, type CircularBuffer } from "./circular";

export const DECISION_HISTORY_LIMIT = 50;

const asDecisions = (frame: unknown): StrategyDecision[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is StrategyDecision =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as StrategyDecision).symbol === "string" &&
			typeof (row as StrategyDecision).action === "string",
	);
};

/*
latestStrategyDecisions returns each symbol's newest backend decision so snapshot
surfaces can render the current thesis tick without walking circular history.
*/
export const latestStrategyDecisions = (
	decisions: Record<string, CircularBuffer<StrategyDecision>>,
): StrategyDecision[] =>
	Object.keys(decisions)
		.sort()
		.flatMap((symbol) => {
			const decision = decisions[symbol]?.values().at(-1);

			return decision === undefined ? [] : [decision];
		});

/*
decisionSymbols lists the symbols that currently own decision history so React
can mount stable row shells while direct paint keeps values live.
*/
export const decisionSymbols = (
	decisions: Record<string, CircularBuffer<StrategyDecision>>,
): string[] => Object.keys(decisions).sort();

/*
decisionStore retains backend strategy decisions in bounded circular buffers so
websocket ingest and dashboard paint stay off the React render path.
*/
export const decisionStore = createStore(
	{
		decisions: {} as Record<string, CircularBuffer<StrategyDecision>>,
		version: 0,
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const rows = asDecisions(frame);

				if (rows.length === 0) {
					return prev;
				}

				const decisions = prev.decisions;

				for (const row of rows) {
					if (!decisions[row.symbol]) {
						decisions[row.symbol] = Circular<StrategyDecision>(
							DECISION_HISTORY_LIMIT,
						);
					}

					decisions[row.symbol].push(row);
				}

				return {
					decisions,
					version: prev.version + 1,
					observed: true,
				};
			}),
		reset: () =>
			setState(() => ({
				decisions: {},
				version: 0,
				observed: false,
			})),
	}),
);
