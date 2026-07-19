import type { StrategyDecision } from "#/types/thesis";
import { type CircularBuffer, latestValues } from "./circular";
import { createKeyedStore } from "./store";

export const DECISION_HISTORY_LIMIT = 50;

/*
latestStrategyDecisions returns each symbol's newest backend decision.
*/
export const latestStrategyDecisions = (
	decisions: Record<string, CircularBuffer<StrategyDecision>>,
): StrategyDecision[] => latestValues(decisions);

/*
decisionSymbols lists symbols that currently own decision history.
*/
export const decisionSymbols = (
	decisions: Record<string, CircularBuffer<StrategyDecision>>,
): string[] => Object.keys(decisions).sort();

/*
decisionStore retains backend strategy decisions per symbol.
*/
export const decisionStore = createKeyedStore<StrategyDecision>()(
	"decisions",
	DECISION_HISTORY_LIMIT,
	(row) => row.symbol,
);
