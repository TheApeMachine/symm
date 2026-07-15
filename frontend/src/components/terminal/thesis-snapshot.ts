import { categoriesStore, categoryValues } from "#/collections/categories";
import { decisionStore } from "#/collections/decisions";
import { findingsList, findingsStore } from "#/collections/findings";
import { forecastsStore, forecastValues } from "#/collections/forecasts";
import { graphsStore, latestGraphFrame } from "#/collections/graphs";
import { hypothesesStore, hypothesisValues } from "#/collections/hypotheses";
import { lifecycleStore } from "#/collections/lifecycle";
import {
	flattenMeasurementBuffer,
	measurementsStore,
} from "#/collections/measurements";
import { tickStore } from "#/collections/tick";
import {
	tradeJournalStore,
	tradeJournalValues,
} from "#/collections/trade-journal";
import type { Category, Measurement } from "#/types/measurement";
import type {
	Finding,
	GraphFrame,
	StrategyDecision,
	ThesisCategory,
	ThesisForecast,
	ThesisHypothesis,
	TradeObservation,
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
	observations: TradeObservation[];
	findings: Finding[];
};

const tickCount = (frame: Record<string, unknown> | null): number | null => {
	const tick = frame?.count;

	return typeof tick === "number" && Number.isFinite(tick) ? tick : null;
};

const measurementsForSymbol = (symbol: string): Measurement[] => {
	const sources = measurementsStore.state.measurements[symbol];

	if (sources === undefined) {
		return [];
	}

	return Object.values(sources).flatMap((buffer) =>
		flattenMeasurementBuffer(buffer),
	);
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
categoriesForSymbol merges thesis.Categories wire rows with legacy measurement
categories so classification evidence survives partial thesis tick frames.
*/
export const categoriesForSymbol = (symbol: string): ThesisCategory[] => {
	const measurements = measurementsForSymbol(symbol);
	const merged = new Map<string, ThesisCategory>();

	for (const category of categoryValues(categoriesStore.state.categories)) {
		if (
			category.symbol !== undefined &&
			category.symbol !== "" &&
			category.symbol !== symbol
		) {
			continue;
		}

		merged.set(category.type, category);
	}

	for (const measurement of measurements) {
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
		observations:
			incoming.observations.length >= previous.observations.length
				? incoming.observations
				: previous.observations,
		findings:
			incoming.findings.length >= previous.findings.length
				? incoming.findings
				: previous.findings,
	};
};

export const thesisSnapshotFor = (symbol: string): ThesisSnapshot => {
	const decision =
		decisionStore.state.decisions[symbol]?.values().at(-1) ?? null;
	const measurements = measurementsForSymbol(symbol);

	return {
		symbol,
		tick: tickCount(tickStore.state.frame),
		lifecycle: lifecycleStore.state.lifecycle[symbol] ?? null,
		graph: latestGraphFrame(graphsStore.state.graphs, symbol),
		measurements,
		decision,
		forecasts: forecastValues(forecastsStore.state.forecasts).filter(
			(forecast) => forecast.symbol === symbol,
		),
		hypotheses: hypothesisValues(hypothesesStore.state.hypotheses).filter(
			(hypothesis) => hypothesis.symbol === symbol,
		),
		categories: categoriesForSymbol(symbol),
		observations: tradeJournalValues(tradeJournalStore.state.journal).filter(
			(observation) => observation.symbol === symbol,
		),
		findings: findingsList(findingsStore.state.findings).filter(
			(finding) => finding.symbol === symbol,
		),
	};
};
