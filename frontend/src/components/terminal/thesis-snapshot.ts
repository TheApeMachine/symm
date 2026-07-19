import type { Holding } from "#/collections/types";
import type { Category, Measurement } from "#/types/measurement";
import type {
	Finding,
	GraphFrame,
	StrategyDecision,
	ThesisCategory,
	ThesisForecast,
	ThesisHypothesis,
} from "#/types/thesis";

export type ThesisSnapshot = {
	symbol: string;
	tick: number | null;
	lifecycle: string | null;
	graph: GraphFrame | null;
	measurements: Measurement[];
	decision: StrategyDecision | null;
	forecasts: ThesisForecast[];
	hypotheses: ThesisHypothesis[];
	categories: ThesisCategory[];
	holdings: Holding[];
	findings: Finding[];
};

/*
ThesisSnapshotInput is the flat buffer material needed to assemble one modal
snapshot without reading frame stores.
*/
export type ThesisSnapshotInput = {
	symbol: string;
	tick: number | null;
	lifecycle: string | null;
	graph: GraphFrame | null;
	measurements: Measurement[];
	decision: StrategyDecision | null;
	forecasts: ThesisForecast[];
	hypotheses: ThesisHypothesis[];
	categories: Array<Category & { symbol?: string }>;
	holdings: Holding[];
	findings: Finding[];
};

const categoryFromMeasurement = (
	symbol: string,
	category: Category,
): ThesisCategory => ({
	symbol,
	type: category.type,
	confidence: category.confidence,
	surprisal: category.surprisal,
	strength: category.strength,
	maturity: 0,
});

/*
categoriesForSymbol merges thesis category wire rows with legacy measurement
categories so classification evidence survives partial thesis tick frames.
*/
export const categoriesForSymbol = (
	symbol: string,
	categories: Array<Category & { symbol?: string }>,
	measurements: Measurement[],
): ThesisCategory[] => {
	const merged = new Map<string, ThesisCategory>();

	for (const category of categories) {
		if (
			category.symbol !== undefined &&
			category.symbol !== "" &&
			category.symbol !== symbol
		) {
			continue;
		}

		merged.set(category.type, {
			symbol,
			type: category.type,
			confidence: category.confidence,
			surprisal: category.surprisal,
			strength: category.strength,
			maturity: 0,
		});
	}

	for (const measurement of measurements) {
		if (measurement.symbol !== symbol) {
			continue;
		}

		for (const category of measurement.categories ?? []) {
			merged.set(
				category.type,
				categoryFromMeasurement(measurement.symbol, category),
			);
		}
	}

	return [...merged.values()];
};

/*
accumulateThesisSnapshot merges live thesis evidence into the modal's retained
view so rail sections never regress to empty between websocket ticks.
*/
export const accumulateThesisSnapshot = (
	previous: ThesisSnapshot | null,
	incoming: ThesisSnapshot,
): ThesisSnapshot => {
	if (previous === null || previous.symbol !== incoming.symbol) {
		return incoming;
	}

	return {
		...incoming,
		lifecycle: incoming.lifecycle ?? previous.lifecycle,
		graph: incoming.graph ?? previous.graph,
		decision: incoming.decision ?? previous.decision,
		forecasts:
			incoming.forecasts.length > 0 ? incoming.forecasts : previous.forecasts,
		hypotheses:
			incoming.hypotheses.length > 0
				? incoming.hypotheses
				: previous.hypotheses,
		categories:
			incoming.categories.length > 0
				? incoming.categories
				: previous.categories,
		holdings:
			incoming.holdings.length >= previous.holdings.length
				? incoming.holdings
				: previous.holdings,
		findings:
			incoming.findings.length >= previous.findings.length
				? incoming.findings
				: previous.findings,
	};
};

/*
thesisSnapshotFor assembles one modal snapshot from already-materialized buffer
rows for the focused symbol.
*/
export const thesisSnapshotFor = (
	input: ThesisSnapshotInput,
): ThesisSnapshot => ({
	symbol: input.symbol,
	tick: input.tick,
	lifecycle: input.lifecycle,
	graph: input.graph,
	measurements: input.measurements.filter(
		(measurement) => measurement.symbol === input.symbol,
	),
	decision: input.decision,
	forecasts: input.forecasts.filter(
		(forecast) => forecast.symbol === input.symbol,
	),
	hypotheses: input.hypotheses.filter(
		(hypothesis) => hypothesis.symbol === input.symbol,
	),
	categories: categoriesForSymbol(
		input.symbol,
		input.categories,
		input.measurements,
	),
	holdings: input.holdings.filter(
		(holding) => holding.symbol === input.symbol,
	),
	findings: input.findings.filter(
		(finding) => finding.symbol === input.symbol,
	),
});
