import { describe, expect, it } from "vitest";

import type { HindsightReport } from "#/collections/app";

import {
	hindsightDiagnosticCoverage,
	hindsightLegCaptureRate,
	hindsightLossPct,
	hindsightLossPositions,
	hindsightRealizedPct,
	hindsightValueCaptureRate,
	rankHindsightLossRecommendations,
	rankHindsightLossRootCauses,
	rankHindsightRecommendations,
	rankHindsightRootCauses,
} from "./hindsight-model";

const report: HindsightReport = {
	captureId: 7,
	status: "ready",
	symbols: [],
	priceTheoreticalCeiling: 0.5,
	executableCeiling: 0.42,
	missedPct: 0.3,
	missedLegs: 2,
	totalLegs: 4,
	realizedPct: 0.2,
	lossPct: 0.05,
	lossPositions: 1,
	valueCaptureRate: 0.4,
	legCaptureRate: 0.5,
	diagnosticCoverage: 0.75,
	rootCauses: [
		{
			category: "execution_feasibility",
			impactPct: 0.1,
			occurrences: 1,
			symbols: ["BBB/USD"],
		},
		{
			category: "admission_policy",
			impactPct: 0.2,
			occurrences: 1,
			symbols: ["AAA/USD"],
		},
	],
	recommendations: [
		{
			key: "execution:coverage",
			kind: "fix_execution",
			target: "execution coverage",
			title: "Measure execution coverage",
			action: "Retain depth coverage.",
			rationale: "The desk rejected the quantity.",
			current: 0,
			suggested: 0,
			hasCurrent: false,
			hasSuggested: false,
			confidence: 0.8,
			impactPct: 0.1,
			occurrences: 1,
			symbols: ["BBB/USD"],
		},
		{
			key: "admission:confidence",
			kind: "tune_parameter",
			target: "trading.admission.minimum_confidence",
			title: "Backtest confidence boundary",
			action: "Replay the exact boundary.",
			rationale: "The gate stopped entry.",
			current: 0.5,
			suggested: 0.48,
			hasCurrent: true,
			hasSuggested: true,
			adjustment: "lower",
			confidence: 0.9,
			impactPct: 0.2,
			occurrences: 1,
			symbols: ["AAA/USD"],
		},
	],
	lossRootCauses: [
		{
			category: "whipsaw_stopout",
			impactPct: 0.05,
			occurrences: 1,
			symbols: ["AAA/USD"],
		},
	],
	lossRecommendations: [
		{
			key: "tune_stoploss_buffer",
			kind: "tune_risk",
			target: "trading.risk.stoploss_buffer",
			title: "Tune stoploss buffer",
			action: "Widen stoploss threshold to clear market noise.",
			rationale: "Position stopped out on immediate wick.",
			current: 0.02,
			suggested: 0.035,
			hasCurrent: true,
			hasSuggested: true,
			adjustment: "raise",
			confidence: 0.88,
			impactPct: 0.05,
			occurrences: 1,
			symbols: ["AAA/USD"],
		},
	],
};

describe("hindsight model", () => {
	it("exposes backend-owned summary arithmetic without re-deriving it", () => {
		expect(hindsightRealizedPct(report)).toBe(0.2);
		expect(hindsightLossPct(report)).toBe(0.05);
		expect(hindsightLossPositions(report)).toBe(1);
		expect(hindsightValueCaptureRate(report)).toBe(0.4);
		expect(hindsightLegCaptureRate(report)).toBe(0.5);
		expect(hindsightDiagnosticCoverage(report)).toBe(0.75);
	});

	it("preserves backend recommendation and root-cause ordering for both opportunities and losses", () => {
		expect(rankHindsightRecommendations(report).map(({ key }) => key)).toEqual([
			"execution:coverage",
			"admission:confidence",
		]);
		expect(rankHindsightRootCauses(report).map(({ category }) => category)).toEqual([
			"execution_feasibility",
			"admission_policy",
		]);
		expect(rankHindsightLossRecommendations(report).map(({ key }) => key)).toEqual([
			"tune_stoploss_buffer",
		]);
		expect(rankHindsightLossRootCauses(report).map(({ category }) => category)).toEqual([
			"whipsaw_stopout",
		]);
	});

	it("renders unavailable backend verdicts as empty rather than synthesizing them", () => {
		const incomplete = {
			...report,
			recommendations: undefined,
			rootCauses: undefined,
			lossRecommendations: undefined,
			lossRootCauses: undefined,
			lossPct: undefined,
			lossPositions: undefined,
		};
		expect(rankHindsightRecommendations(incomplete)).toEqual([]);
		expect(rankHindsightRootCauses(incomplete)).toEqual([]);
		expect(rankHindsightLossRecommendations(incomplete)).toEqual([]);
		expect(rankHindsightLossRootCauses(incomplete)).toEqual([]);
		expect(hindsightLossPct(incomplete)).toBe(0);
		expect(hindsightLossPositions(incomplete)).toBe(0);
	});
});
