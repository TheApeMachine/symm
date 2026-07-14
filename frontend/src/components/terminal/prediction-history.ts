import type { TerminalPredictionSample } from "#/components/terminal/charts";

const PREDICTION_HISTORY_LIMIT = 130;
const historyBySymbol = new Map<string, TerminalPredictionSample[]>();

/*
appendPredictionSample retains one symbol's resonance history outside React so
live canvas paints can update without setState churn.
*/
export const appendPredictionSample = (
	symbol: string,
	sample: TerminalPredictionSample | null,
): TerminalPredictionSample[] => {
	if (sample === null) {
		return historyBySymbol.get(symbol) ?? [];
	}

	const history = historyBySymbol.get(symbol) ?? [];
	const last = history.at(-1);

	if (last?.key === sample.key) {
		return history;
	}

	const next = [...history, sample].slice(-PREDICTION_HISTORY_LIMIT);
	historyBySymbol.set(symbol, next);

	return next;
};
