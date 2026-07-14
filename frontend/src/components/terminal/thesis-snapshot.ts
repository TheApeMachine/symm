import { categoriesStore } from "#/collections/categories";
import { decisionStore } from "#/collections/decisions";
import { findingsList, findingsStore } from "#/collections/findings";
import { forecastsStore } from "#/collections/forecasts";
import { graphsStore } from "#/collections/graphs";
import { hypothesesStore } from "#/collections/hypotheses";
import { lifecycleStore } from "#/collections/lifecycle";
import {
	flattenMeasurementBuffer,
	measurementsStore,
} from "#/collections/measurements";
import {
	categoryRowKey,
	forecastRowKey,
	hypothesisRowKey,
	mergeSnapshotArray,
} from "#/collections/snapshot-retain";
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
	const fromMeasurements = measurements.flatMap((measurement) =>
		(measurement.categories ?? []).map((category) =>
			categoryFromMeasurement(measurement.symbol, category),
		),
	);
	const fromStore = categoriesStore.state.categories.filter(
		(category) =>
			category.symbol === undefined ||
			category.symbol === "" ||
			category.symbol === symbol,
	);

	return mergeSnapshotArray(
		[...fromStore, ...fromMeasurements],
		[],
		categoryRowKey,
	);
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

	const forSymbol = <T extends { symbol: string }>(rows: T[]): T[] =>
		rows.filter((row) => row.symbol === incoming.symbol);

	return {
		...incoming,
		lifecycle: incoming.lifecycle ?? previous.lifecycle,
		graph: incoming.graph ?? previous.graph,
		decision: incoming.decision ?? previous.decision,
		forecasts:
			incoming.forecasts.length > 0
				? mergeSnapshotArray(
						incoming.forecasts,
						forSymbol(previous.forecasts),
						forecastRowKey,
					)
				: previous.forecasts,
		hypotheses:
			incoming.hypotheses.length > 0
				? mergeSnapshotArray(
						incoming.hypotheses,
						forSymbol(previous.hypotheses),
						hypothesisRowKey,
					)
				: previous.hypotheses,
		categories:
			incoming.categories.length > 0
				? mergeSnapshotArray(
						incoming.categories,
						previous.categories,
						categoryRowKey,
					)
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
		graph: graphsStore.state.graphs[symbol] ?? null,
		measurements,
		decision,
		forecasts: forecastsStore.state.forecasts.filter(
			(forecast) => forecast.symbol === symbol,
		),
		hypotheses: hypothesesStore.state.hypotheses.filter(
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
