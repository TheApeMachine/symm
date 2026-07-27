import type {
	Holding,
	Measurement,
	MeasurementCategory,
	Thesis,
	CategoryGraph,
} from "#/collections/types";
import type {
	Finding,
	Graph,
	StrategyDecision,
	ThesisCategory as Category,
	ThesisCategory,
	ThesisForecast,
	ThesisHypothesis,
} from "#/types/thesis";

export type ThesisSnapshot = {
	symbol: string;
	tick: number | null;
	lifecycle: string | null;
	graph: Graph | null;
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
	graph: Graph | null;
	measurements: Measurement[];
	decision: StrategyDecision | null;
	forecasts: ThesisForecast[];
	hypotheses: ThesisHypothesis[];
	categories: Array<Category & { symbol?: string }>;
	holdings: Holding[];
	findings: Finding[];
};

const graphFromCategory = (
	symbol: string,
	graph: CategoryGraph | undefined,
): Graph | null => {
	if (graph === undefined) {
		return null;
	}

	return {
		symbol,
		at: graph.edges.at(-1)?.at ?? graph.nodes.at(-1)?.at ?? new Date(0).toISOString(),
		nodes: graph.nodes
			.filter((node) => node.symbol === symbol)
			.map((node) => ({
				key: `${node.symbol}/${node.type}`,
				kind: "category",
				category: node.type,
				measurement: {
					strength: node.strength,
					freshness: node.freshness,
					at: node.at,
				},
			})),
		edges: graph.edges
			.filter((edge) => edge.symbol === symbol)
			.map((edge) => ({
				from: `${edge.symbol}/${edge.from}`,
				to: `${edge.symbol}/${edge.to}`,
				type: edge.type,
				at: edge.at,
				observedFrom: edge.at,
			})),
	};
};

const categoryFromMeasurement = (
	symbol: string,
	category: MeasurementCategory,
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
			maturity:
				"maturity" in category && typeof category.maturity === "number"
					? category.maturity
					: 0,
		});
	}

	for (const measurement of measurements) {
		if (measurement.symbol !== symbol) {
			continue;
		}

		for (const category of measurement.categories ?? []) {
			const existing = merged.get(category.type);
			const measured = categoryFromMeasurement(measurement.symbol, category);

			if (existing === undefined) {
				merged.set(category.type, measured);
				continue;
			}

			merged.set(category.type, {
				...measured,
				maturity: existing.maturity,
			});
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
		tick: incoming.tick ?? previous.tick,
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
			incoming.holdings.length > 0 ? incoming.holdings : previous.holdings,
		findings:
			incoming.findings.length > 0 ? incoming.findings : previous.findings,
		measurements:
			incoming.measurements.length > 0
				? incoming.measurements
				: previous.measurements,
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
	holdings: input.holdings.filter((holding) => holding.symbol === input.symbol),
	findings: input.findings.filter((finding) => finding.symbol === input.symbol),
});

export const thesisSnapshotFromPersisted = (
	thesis: Thesis,
	symbol: string,
): ThesisSnapshot => ({
	symbol,
	tick: thesis.tick ?? null,
	lifecycle: thesis.lifecycle?.[symbol] ?? null,
	graph: graphFromCategory(symbol, thesis.graphs?.categories),
	measurements: [],
	decision: (thesis.decisions ?? []).find((decision) => decision.symbol === symbol) ?? null,
	forecasts: (thesis.forecasts ?? []).filter((forecast) => forecast.symbol === symbol),
	hypotheses: (thesis.hypotheses ?? []).filter((hypothesis) => hypothesis.symbol === symbol),
	categories: thesis.categories?.[symbol] ?? [],
	holdings: thesis.holdings?.[symbol] ? [thesis.holdings[symbol]] : [],
	findings: (thesis.findings ?? []).filter((finding) => finding.symbol === symbol),
});
